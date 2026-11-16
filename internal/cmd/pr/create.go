package pr

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/git"
)

type createOptions struct {
	repoFlag, title, body, bodyFile, base, head, milestone string
	assignees, labels, reviewers, projects                 []string
	draft, fill, fillFirst, web, editor, dryRun, noMaint   bool
}

func newCreate(f *cmdutil.Factory) *cmdutil.Command {
	o := &createOptions{}
	c := &cmdutil.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: `Create a pull request on ErvisioRepo.

When the current branch isn't fully pushed to a git remote, a prompt will
ask where to push the branch and offer an option to fork the base
repository (forks are not supported by ErvisioRepo). Use '--head' to
explicitly skip any forking or pushing behavior.

A prompt will also ask for the title and the body of the pull request.
Use '--title' and '--body' to skip this, or use '--fill' to autofill
these values from git commits.

Link an issue to the pull request by referencing the issue in the body of
the pull request. If the body text mentions 'Fixes #123' or 'Closes #123',
the referenced issue will automatically get closed when the pull request
gets merged.

By default, users with write access to the base repository can push new
commits to the head branch of the pull request. Disable this with
'--no-maintainer-edit'.`,
		Aliases: []string{"new"},
		GroupID: "general",
		Example: `$ eg pr create --title "The bug is fixed" --body "Everything works again"
$ eg pr create --reviewer monalisa,hubot
$ eg pr create --base develop --head monalisa:feature
$ eg pr create --fill --draft`,
		Flags: cmdutil.NewFlagSet("create"),
		Args:  cmdutil.NoArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &o.repoFlag)
	fl.StringArrayVarP(&o.assignees, "assignee", "a", "Assign people by their `login`. Use \"@me\" to self-assign.")
	fl.StringVarP(&o.base, "base", "B", "", "The `branch` into which you want your code merged")
	fl.StringVarP(&o.body, "body", "b", "", "Body for the pull request")
	fl.StringVarP(&o.bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	fl.BoolVarP(&o.draft, "draft", "d", false, "Mark pull request as a draft")
	fl.BoolVarP(&o.dryRun, "dry-run", "", false, "Print details instead of creating the PR. May still push git changes.")
	fl.BoolVarP(&o.editor, "editor", "e", false, "Skip prompts and open the text editor to write the title and body in. The first line is the title and the remaining text is the body.")
	fl.BoolVarP(&o.fill, "fill", "f", false, "Use commit info for title and body")
	fl.BoolVarP(&o.fillFirst, "fill-first", "", false, "Use first commit info for title and body")
	fl.StringVarP(&o.head, "head", "H", "", "The `branch` that contains commits for your pull request (default [current branch])")
	fl.StringArrayVarP(&o.labels, "label", "l", "Add labels by `name`")
	fl.StringVarP(&o.milestone, "milestone", "m", "", "Add the pull request to a milestone by `name`")
	fl.BoolVarP(&o.noMaint, "no-maintainer-edit", "", false, "Disable maintainer's ability to modify pull request")
	fl.StringArrayVarP(&o.projects, "project", "p", "Add the pull request to projects by `title` (not supported)")
	fl.StringArrayVarP(&o.reviewers, "reviewer", "r", "Request reviews from people by their `login`")
	fl.StringVarP(&o.title, "title", "t", "", "Title for the pull request")
	fl.BoolVarP(&o.web, "web", "w", false, "Open the web browser to create a pull request")
	c.Run = func(_ *cmdutil.Command, _ []string) error { return runCreate(f, fl, o) }
	return c
}

func runCreate(f *cmdutil.Factory, fl *cmdutil.FlagSet, o *createOptions) error {
	ios := f.IO
	if len(o.projects) > 0 {
		return cmdutil.Unsupported("Adding pull requests to projects", "projects")
	}
	for _, r := range shared.SplitList(o.reviewers) {
		if strings.Contains(r, "/") {
			return cmdutil.Unsupported("Team reviewers", "team_reviewers in POST /repos/{owner}/{repo}/pulls/{number}/requested_reviewers")
		}
	}
	if fl.Changed("body") && o.bodyFile != "" {
		return cmdutil.FlagErrorf("specify only one of `--body` or `--body-file`")
	}
	if o.fill && o.fillFirst {
		return cmdutil.FlagErrorf("`--fill` is not supported with `--fill-first`")
	}
	if o.web && (o.editor || o.dryRun) {
		return cmdutil.FlagErrorf("the `--web` flag is not supported with `--editor` or `--dry-run`")
	}
	if o.bodyFile != "" {
		b, err := shared.ReadBodyFile(ios, o.bodyFile)
		if err != nil {
			return err
		}
		o.body = b
		fl.Set("body", b) //nolint:errcheck // known flag
	}
	haveText := o.title != "" && fl.Changed("body")
	autofill := o.fill || o.fillFirst
	if !ios.CanPrompt() && !haveText && !autofill && !o.web && !o.editor {
		return cmdutil.FlagErrorf("must provide `--title` and `--body` (or `--fill` or `--fill-first`) when not running interactively")
	}
	r, err := f.BaseRepo(o.repoFlag)
	if err != nil {
		return err
	}
	client, err := f.Client(r.Host)
	if err != nil {
		return err
	}
	if o.base == "" {
		var repo struct {
			DefaultBranch string `json:"default_branch"`
		}
		if _, err := client.REST(http.MethodGet, shared.RepoPath(r), nil, &repo); err != nil {
			return err
		}
		o.base = repo.DefaultBranch
	}

	g := f.Git("")
	headOwner, headBranch := r.Owner, ""
	var pushRemote *git.Remote
	needsPush := false
	if o.head != "" {
		if own, br, ok := strings.Cut(o.head, ":"); ok {
			headOwner, headBranch = own, br
		} else {
			headBranch = o.head
		}
	} else {
		if headBranch, err = g.CurrentBranch(); err != nil {
			return err
		}
		rm, owner, err := pushTarget(f, r, headBranch)
		if err != nil {
			return err
		}
		pushRemote, headOwner = &rm, owner
		needsPush = !pushedUpToDate(f, rm.Name, headBranch)
	}
	if headBranch == o.base && strings.EqualFold(headOwner, r.Owner) {
		return fmt.Errorf("must be on a branch named differently than %q", o.base)
	}
	head := headBranch
	if !strings.EqualFold(headOwner, r.Owner) {
		head = headOwner + ":" + headBranch
	}
	if existing, err := pullForBranch(client, r, headOwner, headBranch); err == nil && existing != nil && existing.State == "open" && existing.Base.Ref == o.base {
		return fmt.Errorf("a pull request for branch %q into branch %q already exists:\n%s", head, o.base, existing.HTMLURL)
	}

	if autofill {
		t, b, err := fillFromCommits(f, r, pushRemote, o.base, headBranch, o.fillFirst)
		if err != nil {
			return err
		}
		if o.title == "" {
			o.title = t
		}
		if !fl.Changed("body") {
			o.body = b
		}
	}

	if ios.IsStderrTTY() && !o.web {
		msg := "Creating pull request"
		if o.draft {
			msg = "Creating draft pull request"
		}
		if o.dryRun {
			msg = "Dry Running pull request creation"
		}
		fmt.Fprintf(ios.ErrOut, "\n%s for %s into %s in %s\n\n", msg, ios.ColorScheme().Cyan(head), ios.ColorScheme().Cyan(o.base), r.FullName())
	}

	action := "submit"
	switch {
	case o.editor:
		text, err := f.Edit("PULLREQ_EDITMSG.md", o.title+"\n\n"+o.body)
		if err != nil {
			return err
		}
		t, b, _ := strings.Cut(strings.TrimLeft(text, "\n"), "\n")
		o.title, o.body = strings.TrimSpace(t), strings.TrimSpace(b)
	case o.web:
		action = "web"
	case ios.CanPrompt() && !haveText && !autofill:
		p := &cmdutil.Prompter{IO: ios}
		def := o.title
		if def == "" {
			def, _, _ = fillFromCommits(f, r, pushRemote, o.base, headBranch, false)
		}
		if o.title, err = p.Input("Title", def); err != nil {
			return err
		}
		if !fl.Changed("body") {
			if o.body, err = shared.EditorPrompt(f, "Body", o.body); err != nil {
				return err
			}
		}
		options := []string{"Submit", "Submit as draft", "Continue in browser", "Cancel"}
		i, err := p.Select("What's next?", 0, options)
		if err != nil {
			return err
		}
		switch i {
		case 1:
			o.draft = true
		case 2:
			action = "web"
		case 3:
			fmt.Fprintln(ios.ErrOut, "Discarding.")
			return cmdutil.ErrCancel
		}
	}

	if needsPush && pushRemote != nil {
		if err := pushBranch(f, *pushRemote, headBranch); err != nil {
			return err
		}
	}

	if action == "web" {
		q := url.Values{"base": {o.base}, "head": {head}}
		return f.OpenURL(f.WebURL(r, "compare") + "?" + q.Encode())
	}
	if strings.TrimSpace(o.title) == "" {
		return errors.New("title can't be blank")
	}
	if o.dryRun {
		out := ios.Out
		fmt.Fprintf(out, "Would have created a Pull Request with:\n")
		fmt.Fprintf(out, "title:\t%s\ndraft:\t%v\nbase:\t%s\nhead:\t%s\n", o.title, o.draft, o.base, head)
		fmt.Fprintf(out, "labels:\t%s\nreviewers:\t%s\nassignees:\t%s\nmilestone:\t%s\nmaintainerCanModify:\t%v\nbody:\n%s\n",
			strings.Join(shared.SplitList(o.labels), ", "), strings.Join(shared.SplitList(o.reviewers), ", "),
			strings.Join(shared.SplitList(o.assignees), ", "), o.milestone, !o.noMaint, o.body)
		return nil
	}

	req := map[string]any{"title": o.title, "body": o.body, "head": head, "base": o.base, "draft": o.draft,
		"maintainer_can_modify": !o.noMaint}
	var pr shared.PullRequest
	if _, err := client.REST(http.MethodPost, shared.RepoPath(r)+"/pulls", req, &pr); err != nil {
		return err
	}
	if err := addMetadata(client, r, pr.Number, o); err != nil {
		fmt.Fprintln(ios.Out, pr.HTMLURL)
		return err
	}
	fmt.Fprintln(ios.Out, pr.HTMLURL)
	return nil
}

func addMetadata(c *api.Client, r ghrepo.Repo, n int, o *createOptions) error {
	issue := fmt.Sprintf("%s/issues/%d", shared.RepoPath(r), n)
	if ls := shared.SplitList(o.labels); len(ls) > 0 {
		if _, err := c.REST(http.MethodPost, issue+"/labels", map[string]any{"labels": ls}, nil); err != nil {
			return err
		}
	}
	if len(o.assignees) > 0 {
		logins, err := shared.ReplaceMe(c, o.assignees)
		if err != nil {
			return err
		}
		if _, err := c.REST(http.MethodPost, issue+"/assignees", map[string]any{"assignees": logins}, nil); err != nil {
			return err
		}
	}
	if o.milestone != "" {
		m, err := shared.MilestoneNumber(c, r, o.milestone)
		if err != nil {
			return err
		}
		if _, err := c.REST(http.MethodPatch, issue, map[string]any{"milestone": m}, nil); err != nil {
			return err
		}
	}
	if len(o.reviewers) > 0 {
		logins, err := shared.ReplaceMe(c, o.reviewers)
		if err != nil {
			return err
		}
		p := fmt.Sprintf("%s/pulls/%d/requested_reviewers", shared.RepoPath(r), n)
		if _, err := c.REST(http.MethodPost, p, map[string]any{"reviewers": logins}, nil); err != nil {
			return err
		}
	}
	return nil
}

// pushTarget returns the remote the branch is pushed to and the owner of
// its repository: the branch push remote, remote.pushDefault, the branch
// remote, else the remote of the base repository.
func pushTarget(f *cmdutil.Factory, base ghrepo.Repo, branch string) (git.Remote, string, error) {
	g := f.Git("")
	remotes, err := g.Remotes()
	if err != nil {
		return git.Remote{}, "", err
	}
	cfg, err := f.Config()
	if err != nil {
		return git.Remote{}, "", err
	}
	for _, key := range []string{"branch." + branch + ".pushRemote", "remote.pushDefault", "branch." + branch + ".remote"} {
		name := g.ConfigGet(key)
		for _, rm := range remotes {
			if rm.Name != name {
				continue
			}
			if rr, ok := cmdutil.RemoteRepo(cfg.Hosts(), rm.FetchURL); ok {
				return rm, rr.Owner, nil
			}
		}
	}
	rm, err := baseRemote(f, base)
	if err != nil {
		return git.Remote{}, "", fmt.Errorf("could not find a git remote to push %q to; push the branch and use --head: %w", branch, err)
	}
	return rm, base.Owner, nil
}

// pushedUpToDate reports whether remote/branch exists and contains HEAD.
func pushedUpToDate(f *cmdutil.Factory, remote, branch string) bool {
	g := f.Git("")
	tracking := "refs/remotes/" + remote + "/" + branch
	if _, err := g.Output("rev-parse", "--verify", "--quiet", tracking); err != nil {
		return false
	}
	out, err := g.Output("rev-list", "--count", tracking+"..HEAD")
	return err == nil && strings.TrimSpace(out) == "0"
}

// pushBranch pushes the branch, asking first on a terminal.
func pushBranch(f *cmdutil.Factory, rm git.Remote, branch string) error {
	ios := f.IO
	if ios.CanPrompt() {
		p := &cmdutil.Prompter{IO: ios}
		i, err := p.Select(fmt.Sprintf("Where should we push the '%s' branch?", branch), 0,
			[]string{"Push to " + rm.Name + " (" + rm.FetchURL + ")", "Skip pushing the branch", "Cancel"})
		if err != nil {
			return err
		}
		switch i {
		case 1:
			return nil
		case 2:
			return cmdutil.ErrCancel
		}
	}
	return f.Git("").Run("push", "--set-upstream", rm.Name, "HEAD:refs/heads/"+branch)
}

// fillFromCommits builds a title and body from the commits between the
// base branch and HEAD, as gh's --fill and --fill-first.
func fillFromCommits(f *cmdutil.Factory, r ghrepo.Repo, pushRemote *git.Remote, base, head string, first bool) (string, string, error) {
	g := f.Git("")
	baseRef := base
	remote := ""
	if rm, err := baseRemote(f, r); err == nil {
		remote = rm.Name
	} else if pushRemote != nil {
		remote = pushRemote.Name
	}
	if remote != "" {
		if _, err := g.Output("rev-parse", "--verify", "--quiet", "refs/remotes/"+remote+"/"+base); err == nil {
			baseRef = remote + "/" + base
		}
	}
	headRef := "HEAD"
	if cur, err := g.CurrentBranch(); err != nil || cur != head {
		headRef = head
	}
	out, err := g.Output("-c", "log.ShowSignature=false", "log", "--reverse", "--format=%H", baseRef+".."+headRef)
	if err != nil {
		return "", "", fmt.Errorf("could not compute title or body defaults: %w", err)
	}
	shas := strings.Fields(out)
	if len(shas) == 0 {
		return humanize(head), "", nil
	}
	message := func(sha string) (string, string) {
		subj, _ := g.Output("log", "-1", "--format=%s", sha)
		body, _ := g.Output("log", "-1", "--format=%b", sha)
		return strings.TrimSpace(subj), strings.TrimSpace(body)
	}
	if first || len(shas) == 1 {
		t, b := message(shas[0])
		return t, b, nil
	}
	var lines []string
	for _, sha := range shas {
		t, _ := message(sha)
		lines = append(lines, "- "+t)
	}
	return humanize(head), strings.Join(lines, "\n"), nil
}

// humanize turns "fix-the-bug" into "Fix the bug".
func humanize(branch string) string {
	s := strings.NewReplacer("-", " ", "_", " ").Replace(branch)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
