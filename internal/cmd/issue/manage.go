package issue

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

func issueRef(r ghrepo.Repo, i *shared.Issue) string {
	return fmt.Sprintf("%s#%d (%s)", r.FullName(), i.Number, i.Title)
}

func newClose(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag, comment, reason, duplicateOf string
	c := &cmdutil.Command{
		Use:     "close {<number> | <url>}",
		Short:   "Close issue",
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("close"),
		Args:    cmdutil.ExactArgs(1, "argument required: issue number or URL"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.StringVarP(&comment, "comment", "c", "", "Leave a closing comment")
	c.Flags.EnumVarP(&reason, "reason", "r", "", []string{"completed", "not planned", "duplicate"}, "Reason for closing")
	c.Flags.StringVarP(&duplicateOf, "duplicate-of", "", "", "Mark as duplicate of another issue by `number` or URL")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if duplicateOf != "" {
			if reason != "" && reason != "duplicate" {
				return cmdutil.FlagErrorf("`--duplicate-of` can only be used with `--reason duplicate`")
			}
			reason = "duplicate"
		}
		r, n, err := shared.Target(f, repoFlag, args[0])
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		issue, err := shared.FetchIssue(client, r, n)
		if err != nil {
			return err
		}
		ios := f.IO
		cs := ios.ColorScheme()
		if issue.State == "closed" {
			fmt.Fprintf(ios.ErrOut, "%s Issue %s is already closed\n", cs.Yellow("!"), issueRef(r, issue))
			return nil
		}
		if comment != "" {
			if _, err := client.REST(http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", shared.RepoPath(r), n), map[string]any{"body": comment}, nil); err != nil {
				return err
			}
		}
		if duplicateOf != "" {
			dn, _, err := shared.ParseNumberArg(duplicateOf)
			if err != nil {
				return err
			}
			body := fmt.Sprintf("Duplicate of #%d", dn)
			if _, err := client.REST(http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", shared.RepoPath(r), n), map[string]any{"body": body}, nil); err != nil {
				return err
			}
		}
		patch := map[string]any{"state": "closed"}
		if reason != "" {
			patch["state_reason"] = strings.ReplaceAll(reason, " ", "_")
		}
		if _, err := client.REST(http.MethodPatch, fmt.Sprintf("%s/issues/%d", shared.RepoPath(r), n), patch, nil); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Closed issue %s\n", cs.Magenta("✓"), issueRef(r, issue))
		}
		return nil
	}
	return c
}

func newReopen(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag, comment string
	c := &cmdutil.Command{
		Use:     "reopen {<number> | <url>}",
		Short:   "Reopen issue",
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("reopen"),
		Args:    cmdutil.ExactArgs(1, "argument required: issue number or URL"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.StringVarP(&comment, "comment", "c", "", "Add a reopening comment")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, n, err := shared.Target(f, repoFlag, args[0])
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		issue, err := shared.FetchIssue(client, r, n)
		if err != nil {
			return err
		}
		ios := f.IO
		cs := ios.ColorScheme()
		if issue.State == "open" {
			fmt.Fprintf(ios.ErrOut, "%s Issue %s is already open\n", cs.Yellow("!"), issueRef(r, issue))
			return nil
		}
		if comment != "" {
			if _, err := client.REST(http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", shared.RepoPath(r), n), map[string]any{"body": comment}, nil); err != nil {
				return err
			}
		}
		if _, err := client.REST(http.MethodPatch, fmt.Sprintf("%s/issues/%d", shared.RepoPath(r), n), map[string]any{"state": "open"}, nil); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Reopened issue %s\n", cs.SuccessIcon(), issueRef(r, issue))
		}
		return nil
	}
	return c
}

func newDelete(*cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	var yes bool
	c := &cmdutil.Command{
		Use:     "delete {<number> | <url>}",
		Short:   "Delete issue",
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("delete"),
		Args:    cmdutil.ExactArgs(1, "argument required: issue number or URL"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&yes, "yes", "", false, "Confirm deletion without prompting")
	c.Run = func(*cmdutil.Command, []string) error {
		return cmdutil.Unsupported("Deleting issues", "the deleteIssue GraphQL mutation")
	}
	return c
}

func newLock(_ *cmdutil.Factory, lock bool) *cmdutil.Command {
	var repoFlag, reason string
	name, short := "lock", "Lock issue conversation"
	if !lock {
		name, short = "unlock", "Unlock issue conversation"
	}
	c := &cmdutil.Command{
		Use:     name + " {<number> | <url>}",
		Short:   short,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet(name),
		Args:    cmdutil.ExactArgs(1, "argument required: issue number or URL"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	if lock {
		c.Flags.EnumVarP(&reason, "reason", "r", "", []string{"off_topic", "resolved", "spam", "too_heated"}, "Optional reason for locking conversation")
	}
	c.Run = func(*cmdutil.Command, []string) error {
		method := "PUT"
		if !lock {
			method = "DELETE"
		}
		return cmdutil.Unsupported("Locking conversations", method+" /repos/{owner}/{repo}/issues/{number}/lock")
	}
	return c
}

func newEdit(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag string
		o        shared.EditOptions
	)
	c := &cmdutil.Command{
		Use:   "edit {<numbers> | <urls>}",
		Short: "Edit issues",
		Long: `Edit one or more issues within the same repository.

Editing issues' projects is not supported by ErvisioRepo.`,
		GroupID: "targeted",
		Example: `$ eg issue edit 23 --title "I found a bug" --body "Nothing works"
$ eg issue edit 23 --add-label "bug,help wanted" --remove-label "core"
$ eg issue edit 23 --add-assignee "@me" --remove-assignee monalisa,hubot
$ eg issue edit 23 --milestone "Version 1"
$ eg issue edit 23 34 --add-label "help wanted"`,
		Flags: cmdutil.NewFlagSet("edit"),
		Args:  cmdutil.RangeArgs(1, 100, "argument required: issue number or URL"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	o.Register(c.Flags)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		fl := c.Flags
		if err := o.Validate(fl); err != nil {
			return err
		}
		if len(args) > 1 && (fl.Changed("title") || fl.Changed("body") || fl.Changed("body-file")) {
			return cmdutil.FlagErrorf("multiple issues cannot be edited with `--title`, `--body` or `--body-file`")
		}
		ios := f.IO
		if !o.Changed(fl) {
			if !ios.CanPrompt() || len(args) > 1 {
				return cmdutil.FlagErrorf("field to edit flag required when not running interactively")
			}
			if err := promptEdit(f, repoFlag, args[0], &o, fl); err != nil {
				return err
			}
		}
		for _, a := range args {
			r, n, err := shared.Target(f, repoFlag, a)
			if err != nil {
				return err
			}
			client, err := f.Client(r.Host)
			if err != nil {
				return err
			}
			if err := o.Apply(f, client, r, n, fl, nil); err != nil {
				return err
			}
			issue, err := shared.FetchIssue(client, r, n)
			if err != nil {
				return err
			}
			fmt.Fprintln(ios.Out, issue.HTMLURL)
		}
		return nil
	}
	return c
}

// promptEdit asks for the title and body of an issue, gh's interactive
// edit limited to line prompts and the editor.
func promptEdit(f *cmdutil.Factory, repoFlag, arg string, o *shared.EditOptions, fl *cmdutil.FlagSet) error {
	r, n, err := shared.Target(f, repoFlag, arg)
	if err != nil {
		return err
	}
	client, err := f.Client(r.Host)
	if err != nil {
		return err
	}
	issue, err := shared.FetchIssue(client, r, n)
	if err != nil {
		return err
	}
	p := &cmdutil.Prompter{IO: f.IO}
	title, err := p.Input("Title", issue.Title)
	if err != nil {
		return err
	}
	body, err := shared.EditorPrompt(f, "Body", issue.BodyText())
	if err != nil {
		return err
	}
	if title != issue.Title {
		o.Title = title
		if err := fl.Set("title", title); err != nil {
			return err
		}
	}
	if body != issue.BodyText() {
		o.Body = body
		if err := fl.Set("body", body); err != nil {
			return err
		}
	}
	return nil
}

func newComment(f *cmdutil.Factory) *cmdutil.Command {
	c := shared.NewCommentCmd(f, "issue", "{<number> | <url>}", cmdutil.ExactArgs(1, "argument required: issue number or URL"),
		func(repoFlag string, args []string) (ghrepo.Repo, int, string, error) {
			r, n, err := shared.Target(f, repoFlag, args[0])
			if err != nil {
				return ghrepo.Repo{}, 0, "", err
			}
			return r, n, f.WebURL(r, fmt.Sprintf("issues/%d", n)), nil
		})
	c.GroupID = "targeted"
	return c
}
