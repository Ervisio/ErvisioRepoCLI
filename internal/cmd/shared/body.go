package shared

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

// ReadBodyFile reads a body from a file, or from stdin for "-".
func ReadBodyFile(ios *iostreams.IOStreams, path string) (string, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(ios.In)
	} else {
		b, err = os.ReadFile(filepath.Clean(path))
	}
	if err != nil {
		return "", fmt.Errorf("read body file: %w", err)
	}
	return string(b), nil
}

// EditorPrompt asks for a long text the way gh's editor prompt does:
// "e" opens the editor, Enter keeps the default.
func EditorPrompt(f *cmdutil.Factory, label, def string) (string, error) {
	ios := f.IO
	cs := ios.ColorScheme()
	editor := f.EditorCommand()
	name := editor
	if i := strings.IndexAny(name, " \t"); i > 0 {
		name = name[:i]
	}
	fmt.Fprintf(ios.ErrOut, "%s %s [(e) to launch %s, enter to skip] ", cs.Green("?"), cs.Bold(label), filepath.Base(name))
	s, err := ios.ReadLine()
	if errors.Is(err, io.EOF) {
		return "", cmdutil.ErrCancel
	}
	if err != nil {
		return "", fmt.Errorf("read answer: %w", err)
	}
	if strings.TrimSpace(strings.ToLower(s)) != "e" {
		return def, nil
	}
	return f.Edit("EG_BODY.md", def)
}

// ---- comments ----

// CommentTarget resolves the repository and number a comment command
// works on.
type CommentTarget func(repoFlag string, args []string) (ghrepo.Repo, int, string, error)

// NewCommentCmd builds `issue comment` and `pr comment`.
func NewCommentCmd(f *cmdutil.Factory, noun string, argUse string, args func([]string) error, target CommentTarget) *cmdutil.Command {
	var (
		repoFlag, body, bodyFile             string
		editor, web, editLast, deleteLast, y bool
		createIfNone                         bool
	)
	c := &cmdutil.Command{
		Use:   "comment " + argUse,
		Short: "Add a comment to " + article(noun),
		Long: fmt.Sprintf(`Add a comment to %s.

Without the body text supplied through flags, the command interactively
opens your editor.`, article(noun)),
		Example: fmt.Sprintf(`$ eg %s comment 12 --body "Hi from eg"`, cmdName(noun)),
		Flags:   cmdutil.NewFlagSet("comment"),
		Args:    args,
	}
	fl := c.Flags
	AddRepoFlag(c, &repoFlag)
	fl.StringVarP(&body, "body", "b", "", "The comment body `text`")
	fl.StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	fl.BoolVarP(&editor, "editor", "e", false, "Skip prompts and open the text editor to write the body in")
	fl.BoolVarP(&web, "web", "w", false, "Open the web browser to write the comment")
	fl.BoolVarP(&editLast, "edit-last", "", false, "Edit the last comment of the current user")
	fl.BoolVarP(&deleteLast, "delete-last", "", false, "Delete the last comment of the current user")
	fl.BoolVarP(&y, "yes", "", false, "Skip the delete confirmation prompt when --delete-last is provided")
	fl.BoolVarP(&createIfNone, "create-if-none", "", false, "Create a new comment if no comments are found. Can be used only with --edit-last")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		n := 0
		for _, b := range []bool{body != "", bodyFile != "", editor, web} {
			if b {
				n++
			}
		}
		if n > 1 {
			return cmdutil.FlagErrorf("specify only one of `--body`, `--body-file`, `--editor`, or `--web`")
		}
		if editLast && deleteLast {
			return cmdutil.FlagErrorf("specify only one of `--edit-last` or `--delete-last`")
		}
		if createIfNone && !editLast {
			return cmdutil.FlagErrorf("`--create-if-none` can only be used with `--edit-last`")
		}
		ios := f.IO
		r, num, htmlURL, err := target(repoFlag, args)
		if err != nil {
			return err
		}
		if web {
			if htmlURL == "" {
				htmlURL = f.WebURL(r, fmt.Sprintf("issues/%d", num))
			}
			return f.OpenURL(htmlURL)
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var last *Comment
		if editLast || deleteLast {
			if last, err = lastOwnComment(client, r, num); err != nil {
				return err
			}
			if last == nil && (!editLast || !createIfNone) {
				return errors.New("no comments found for current user")
			}
		}
		if deleteLast {
			if !y {
				if !ios.CanPrompt() {
					return cmdutil.FlagErrorf("--yes required when not running interactively")
				}
				preview := last.Body
				if len(preview) > 40 {
					preview = preview[:40] + "..."
				}
				ok, err := (&cmdutil.Prompter{IO: ios}).Confirm(fmt.Sprintf("Delete the comment %q?", preview), false)
				if err != nil {
					return err
				}
				if !ok {
					return cmdutil.ErrCancel
				}
			}
			if _, err := client.REST(http.MethodDelete, fmt.Sprintf("%s/issues/comments/%d", RepoPath(r), last.ID), nil, nil); err != nil {
				return err
			}
			if ios.IsStderrTTY() {
				fmt.Fprintf(ios.ErrOut, "%s Comment deleted\n", ios.ColorScheme().SuccessIcon())
			}
			return nil
		}
		text := body
		switch {
		case bodyFile != "":
			if text, err = ReadBodyFile(ios, bodyFile); err != nil {
				return err
			}
		case body == "":
			if !ios.CanPrompt() && !editor {
				return cmdutil.FlagErrorf("`--body`, `--body-file` or `--web` required when not running interactively")
			}
			initial := ""
			if last != nil {
				initial = last.Body
			}
			if text, err = f.Edit("EG_COMMENT.md", initial); err != nil {
				return err
			}
		}
		if strings.TrimSpace(text) == "" {
			return errors.New("aborted: the comment body is empty")
		}
		var out Comment
		if last != nil && editLast {
			_, err = client.REST(http.MethodPatch, fmt.Sprintf("%s/issues/comments/%d", RepoPath(r), last.ID), map[string]any{"body": text}, &out)
		} else {
			_, err = client.REST(http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", RepoPath(r), num), map[string]any{"body": text}, &out)
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(ios.Out, out.HTMLURL)
		return nil
	}
	return c
}

func article(noun string) string {
	if noun == "issue" {
		return "an issue"
	}
	return "a " + noun
}

func cmdName(noun string) string {
	if noun == "issue" {
		return "issue"
	}
	return "pr"
}

// ListComments loads the conversation comments of an issue or pull
// request.
func ListComments(c *api.Client, r ghrepo.Repo, n int) ([]*Comment, error) {
	var all []*Comment
	next := fmt.Sprintf("%s/issues/%d/comments?per_page=100", RepoPath(r), n)
	for next != "" {
		var batch []*Comment
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
	}
	return all, nil
}

func lastOwnComment(c *api.Client, r ghrepo.Repo, n int) (*Comment, error) {
	me, err := ViewerLogin(c)
	if err != nil {
		return nil, err
	}
	list, err := ListComments(c, r, n)
	if err != nil {
		return nil, err
	}
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].User != nil && strings.EqualFold(list[i].User.Login, me) {
			return list[i], nil
		}
	}
	return nil, nil
}

// ExportComments lists comments in gh's JSON shape.
func ExportComments(c *api.Client, r ghrepo.Repo, n int) ([]any, error) {
	list, err := ListComments(c, r, n)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(list))
	for i, cm := range list {
		out[i] = cm.Export()
	}
	return out, nil
}

// WantsField reports whether --json asked for name.
func WantsField(e *cmdutil.Exporter, name string) bool {
	for _, f := range e.Fields() {
		if f == name {
			return true
		}
	}
	return false
}
