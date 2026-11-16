package issue

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

func newCreate(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, title, body, bodyFile, milestone string
		assignees, labels, projects                []string
		web, editor                                bool
	)
	c := &cmdutil.Command{
		Use:   "create",
		Short: "Create a new issue",
		Long: `Create an issue on ErvisioRepo.

Adding an issue to projects requires authorization with the "project"
scope; projects are not supported by ErvisioRepo.`,
		Aliases: []string{"new"},
		GroupID: "general",
		Example: `$ eg issue create --title "I found a bug" --body "Nothing works"
$ eg issue create --label "bug,help wanted"
$ eg issue create --label bug --label "help wanted"
$ eg issue create --assignee monalisa,hubot
$ eg issue create --assignee "@me"
$ eg issue create --milestone "The big 1.0"`,
		Flags: cmdutil.NewFlagSet("create"),
		Args:  cmdutil.NoArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.StringVarP(&title, "title", "t", "", "Supply a title. Will prompt for one otherwise.")
	fl.StringVarP(&body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	fl.StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	fl.StringArrayVarP(&assignees, "assignee", "a", "Assign people by their `login`. Use \"@me\" to self-assign.")
	fl.StringArrayVarP(&labels, "label", "l", "Add labels by `name`")
	fl.StringVarP(&milestone, "milestone", "m", "", "Add the issue to a milestone by `name`")
	fl.StringArrayVarP(&projects, "project", "p", "Add the issue to projects by `title` (not supported)")
	fl.BoolVarP(&web, "web", "w", false, "Open the browser to create an issue")
	fl.BoolVarP(&editor, "editor", "e", false, "Skip prompts and open the text editor to write the title and body in. The first line is the title and the remaining text is the body.")
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		ios := f.IO
		if len(projects) > 0 {
			return cmdutil.Unsupported("Adding issues to projects", "projects")
		}
		if fl.Changed("body") && bodyFile != "" {
			return cmdutil.FlagErrorf("specify only one of `--body` or `--body-file`")
		}
		if editor && web {
			return cmdutil.FlagErrorf("specify only one of `--editor` or `--web`")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		if web {
			return f.OpenURL(f.WebURL(r, "issues/new"))
		}
		if bodyFile != "" {
			if body, err = shared.ReadBodyFile(ios, bodyFile); err != nil {
				return err
			}
		}
		interactive := ios.CanPrompt() && !editor && (title == "" || !fl.Changed("body") && bodyFile == "")
		if !ios.CanPrompt() && !editor && (title == "" || !fl.Changed("body") && bodyFile == "") {
			return cmdutil.FlagErrorf("must provide `--title` and `--body` (or `--body-file`) when not running interactively")
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "\nCreating issue in %s\n\n", r.FullName())
		}
		switch {
		case editor:
			text, err := f.Edit("ISSUE_EDITMSG.md", title+"\n\n"+body)
			if err != nil {
				return err
			}
			title, body, _ = strings.Cut(strings.TrimLeft(text, "\n"), "\n")
			title, body = strings.TrimSpace(title), strings.TrimSpace(body)
		case interactive:
			p := &cmdutil.Prompter{IO: ios}
			if title == "" {
				if title, err = p.Input("Title", ""); err != nil {
					return err
				}
			}
			if !fl.Changed("body") && bodyFile == "" {
				if body, err = shared.EditorPrompt(f, "Body", body); err != nil {
					return err
				}
			}
			i, err := p.Select("What's next?", 0, []string{"Submit", "Cancel"})
			if err != nil {
				return err
			}
			if i == 1 {
				fmt.Fprintln(ios.ErrOut, "Discarding.")
				return cmdutil.ErrCancel
			}
		}
		if strings.TrimSpace(title) == "" {
			return errors.New("title can't be blank")
		}
		req := map[string]any{"title": title, "body": body}
		if ls := shared.SplitList(labels); len(ls) > 0 {
			req["labels"] = ls
		}
		if len(assignees) > 0 {
			logins, err := shared.ReplaceMe(client, assignees)
			if err != nil {
				return err
			}
			req["assignees"] = logins
		}
		if milestone != "" {
			n, err := shared.MilestoneNumber(client, r, milestone)
			if err != nil {
				return err
			}
			req["milestone"] = n
		}
		var out shared.Issue
		if _, err := client.REST(http.MethodPost, shared.RepoPath(r)+"/issues", req, &out); err != nil {
			return err
		}
		fmt.Fprintln(ios.Out, out.HTMLURL)
		return nil
	}
	return c
}
