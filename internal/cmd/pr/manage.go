package pr

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

func addComment(fd *finder, n int, body string) error {
	_, err := fd.client.REST(http.MethodPost, fmt.Sprintf("%s/issues/%d/comments", shared.RepoPath(fd.repo), n), map[string]any{"body": body}, nil)
	return err
}

func setState(fd *finder, n int, patch map[string]any) error {
	_, err := fd.client.REST(http.MethodPatch, fmt.Sprintf("%s/pulls/%d", shared.RepoPath(fd.repo), n), patch, nil)
	return err
}

func newClose(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, comment string
		deleteBranch      bool
	)
	c := &cmdutil.Command{
		Use:     "close {<number> | <url> | <branch>}",
		Short:   "Close a pull request",
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("close"),
		Args:    cmdutil.ExactArgs(1, "argument required: pull request number, URL or branch"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.StringVarP(&comment, "comment", "c", "", "Leave a closing comment")
	c.Flags.BoolVarP(&deleteBranch, "delete-branch", "d", false, "Delete the local and remote branch after close")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		ios := f.IO
		cs := ios.ColorScheme()
		switch {
		case p.StateName() == "MERGED":
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s can't be closed because it was already merged\n", cs.FailureIcon(), prRef(fd.repo, p))
			return cmdutil.ErrSilent
		case p.State == "closed":
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s is already closed\n", cs.WarningIcon(), prRef(fd.repo, p))
			return nil
		}
		if comment != "" {
			if err := addComment(fd, p.Number, comment); err != nil {
				return err
			}
		}
		if err := setState(fd, p.Number, map[string]any{"state": "closed"}); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Closed pull request %s\n", cs.Red("✓"), prRef(fd.repo, p))
		}
		if deleteBranch {
			return deleteBranches(f, fd, p, false)
		}
		return nil
	}
	return c
}

func newReopen(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag, comment string
	c := &cmdutil.Command{
		Use:     "reopen {<number> | <url> | <branch>}",
		Short:   "Reopen a pull request",
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("reopen"),
		Args:    cmdutil.ExactArgs(1, "argument required: pull request number, URL or branch"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.StringVarP(&comment, "comment", "c", "", "Add a reopening comment")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		ios := f.IO
		cs := ios.ColorScheme()
		switch {
		case p.StateName() == "MERGED":
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s can't be reopened because it was already merged\n", cs.FailureIcon(), prRef(fd.repo, p))
			return cmdutil.ErrSilent
		case p.State == "open":
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s is already open\n", cs.WarningIcon(), prRef(fd.repo, p))
			return nil
		}
		if comment != "" {
			if err := addComment(fd, p.Number, comment); err != nil {
				return err
			}
		}
		if err := setState(fd, p.Number, map[string]any{"state": "open"}); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Reopened pull request %s\n", cs.SuccessIcon(), prRef(fd.repo, p))
		}
		return nil
	}
	return c
}

func newReady(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	var undo bool
	c := &cmdutil.Command{
		Use:   "ready " + argUse,
		Short: "Mark a pull request as ready for review",
		Long: `Mark a pull request as ready for review.

Without an argument, the pull request that belongs to the current branch
is marked as ready.

If supported by your plan, convert to draft with --undo.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("ready"),
		Args:    selectorArgs,
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&undo, "undo", "", false, `Convert a pull request to "draft"`)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		ios := f.IO
		cs := ios.ColorScheme()
		if p.State != "open" {
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s is closed. Only draft pull requests can be marked as \"ready for review\"\n", cs.FailureIcon(), prRef(fd.repo, p))
			return cmdutil.ErrSilent
		}
		if undo {
			if p.Draft {
				fmt.Fprintf(ios.ErrOut, "%s Pull request %s is already \"in draft\"\n", cs.WarningIcon(), prRef(fd.repo, p))
				return nil
			}
			if err := setState(fd, p.Number, map[string]any{"draft": true}); err != nil {
				return err
			}
			if ios.IsStderrTTY() {
				fmt.Fprintf(ios.ErrOut, "%s Pull request %s is converted to \"draft\"\n", cs.SuccessIcon(), prRef(fd.repo, p))
			}
			return nil
		}
		if !p.Draft {
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s is already \"ready for review\"\n", cs.WarningIcon(), prRef(fd.repo, p))
			return nil
		}
		if err := setState(fd, p.Number, map[string]any{"draft": false}); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s is marked as \"ready for review\"\n", cs.SuccessIcon(), prRef(fd.repo, p))
		}
		return nil
	}
	return c
}

func newComment(f *cmdutil.Factory) *cmdutil.Command {
	c := shared.NewCommentCmd(f, "pull request", argUse, selectorArgs,
		func(repoFlag string, args []string) (ghrepo.Repo, int, string, error) {
			fd, p, err := find(f, repoFlag, args)
			if err != nil {
				return ghrepo.Repo{}, 0, "", err
			}
			return fd.repo, p.Number, p.HTMLURL, nil
		})
	c.GroupID = "targeted"
	return c
}

func newEdit(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, base                string
		addReviewers, removeReviewers []string
		o                             shared.EditOptions
	)
	c := &cmdutil.Command{
		Use:   "edit " + argUse,
		Short: "Edit a pull request",
		Long: `Edit a pull request.

Without an argument, the pull request that belongs to the current branch
is selected. Team reviewers and projects are not supported by ErvisioRepo.`,
		GroupID: "targeted",
		Example: `$ eg pr edit 23 --title "I found a bug" --body "Nothing works"
$ eg pr edit 23 --add-label "bug,help wanted" --remove-label "core"
$ eg pr edit 23 --add-reviewer monalisa,hubot --remove-reviewer myorg/team-name
$ eg pr edit 23 --add-assignee "@me" --remove-assignee monalisa,hubot
$ eg pr edit 23 --milestone "Version 1"`,
		Flags: cmdutil.NewFlagSet("edit"),
		Args:  selectorArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	o.Register(fl)
	fl.StringVarP(&base, "base", "B", "", "Change the base `branch` for this pull request")
	fl.StringArrayVarP(&addReviewers, "add-reviewer", "", "Add reviewers by their `login`.")
	fl.StringArrayVarP(&removeReviewers, "remove-reviewer", "", "Remove reviewers by their `login`.")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := o.Validate(fl); err != nil {
			return err
		}
		for _, r := range shared.SplitList(append(append([]string{}, addReviewers...), removeReviewers...)) {
			if strings.Contains(r, "/") {
				return cmdutil.Unsupported("Team reviewers", "team_reviewers in POST /repos/{owner}/{repo}/pulls/{number}/requested_reviewers")
			}
		}
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		ios := f.IO
		if !o.Changed(fl) && base == "" && len(addReviewers) == 0 && len(removeReviewers) == 0 {
			if !ios.CanPrompt() {
				return cmdutil.FlagErrorf("field to edit flag required when not running interactively")
			}
			pr := &cmdutil.Prompter{IO: ios}
			title, err := pr.Input("Title", p.Title)
			if err != nil {
				return err
			}
			body, err := shared.EditorPrompt(f, "Body", p.BodyText())
			if err != nil {
				return err
			}
			if title != p.Title {
				if err := fl.Set("title", title); err != nil {
					return err
				}
			}
			if body != p.BodyText() {
				if err := fl.Set("body", body); err != nil {
					return err
				}
			}
		}
		pullPatch := map[string]any{}
		if base != "" {
			pullPatch["base"] = base
		}
		if err := o.Apply(f, fd.client, fd.repo, p.Number, fl, pullPatch); err != nil {
			return err
		}
		reviewers := fmt.Sprintf("%s/pulls/%d/requested_reviewers", shared.RepoPath(fd.repo), p.Number)
		if len(addReviewers) > 0 {
			logins, err := shared.ReplaceMe(fd.client, addReviewers)
			if err != nil {
				return err
			}
			if _, err := fd.client.REST(http.MethodPost, reviewers, map[string]any{"reviewers": logins}, nil); err != nil {
				return err
			}
		}
		if len(removeReviewers) > 0 {
			logins, err := shared.ReplaceMe(fd.client, removeReviewers)
			if err != nil {
				return err
			}
			if _, err := fd.client.REST(http.MethodDelete, reviewers, map[string]any{"reviewers": logins}, nil); err != nil {
				return err
			}
		}
		fmt.Fprintln(ios.Out, p.HTMLURL)
		return nil
	}
	return c
}

func newReview(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, body, bodyFile        string
		approve, requestChanges, commnt bool
	)
	c := &cmdutil.Command{
		Use:   "review " + argUse,
		Short: "Add a review to a pull request",
		Long: `Add a review to a pull request.

Without an argument, the pull request that belongs to the current branch
is reviewed.`,
		GroupID: "targeted",
		Example: `# approve the pull request of the current branch
$ eg pr review --approve

# leave a review comment for the current branch
$ eg pr review --comment -b "interesting"

# add a review for a specific pull request
$ eg pr review 123

# request changes on a specific pull request
$ eg pr review 123 -r -b "needs more ASCII art"`,
		Flags: cmdutil.NewFlagSet("review"),
		Args:  selectorArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.BoolVarP(&approve, "approve", "a", false, "Approve pull request")
	fl.BoolVarP(&requestChanges, "request-changes", "r", false, "Request changes on a pull request")
	fl.BoolVarP(&commnt, "comment", "c", false, "Comment on a pull request")
	fl.StringVarP(&body, "body", "b", "", "Specify the body of a review")
	fl.StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		n := 0
		event := ""
		for _, e := range []struct {
			on   bool
			name string
		}{{approve, "APPROVE"}, {requestChanges, "REQUEST_CHANGES"}, {commnt, "COMMENT"}} {
			if e.on {
				n++
				event = e.name
			}
		}
		if n > 1 {
			return cmdutil.FlagErrorf("need exactly one of --approve, --request-changes, or --comment")
		}
		if fl.Changed("body") && bodyFile != "" {
			return cmdutil.FlagErrorf("specify only one of `--body` or `--body-file`")
		}
		ios := f.IO
		if bodyFile != "" {
			b, err := shared.ReadBodyFile(ios, bodyFile)
			if err != nil {
				return err
			}
			body = b
		}
		if event == "" && !ios.CanPrompt() {
			return cmdutil.FlagErrorf("--approve, --request-changes, or --comment required when not running interactively")
		}
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		if event == "" {
			pr := &cmdutil.Prompter{IO: ios}
			i, err := pr.Select("What kind of review do you want to give?", 0, []string{"Comment", "Approve", "Request changes"})
			if err != nil {
				return err
			}
			event = []string{"COMMENT", "APPROVE", "REQUEST_CHANGES"}[i]
			if body, err = shared.EditorPrompt(f, "Review body", body); err != nil {
				return err
			}
			ok, err := pr.Confirm("Submit?", true)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.ErrCancel
			}
		}
		if event != "APPROVE" && strings.TrimSpace(body) == "" {
			return errors.New("body cannot be blank for comment or request-changes review")
		}
		req := map[string]any{"event": event}
		if body != "" {
			req["body"] = body
		}
		if _, err := fd.client.REST(http.MethodPost, fmt.Sprintf("%s/pulls/%d/reviews", shared.RepoPath(fd.repo), p.Number), req, nil); err != nil {
			return err
		}
		if !ios.IsStderrTTY() {
			return nil
		}
		cs := ios.ColorScheme()
		ref := fmt.Sprintf("%s#%d", fd.repo.FullName(), p.Number)
		switch event {
		case "APPROVE":
			fmt.Fprintf(ios.ErrOut, "%s Approved pull request %s\n", cs.SuccessIcon(), ref)
		case "REQUEST_CHANGES":
			fmt.Fprintf(ios.ErrOut, "%s Requested changes to pull request %s\n", cs.Red("+"), ref)
		default:
			fmt.Fprintf(ios.ErrOut, "%s Reviewed pull request %s\n", cs.Gray("-"), ref)
		}
		return nil
	}
	return c
}
