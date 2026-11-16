// Package pr implements `eg pr`.
package pr

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/git"
)

// New returns the pr command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "pr <command>",
		Short:   "Manage pull requests",
		Long:    "Work with ErvisioRepo pull requests.",
		GroupID: "core",
		Example: `$ eg pr checkout 353
$ eg pr create --fill
$ eg pr view --web`,
	}
	c.AddGroup(cmdutil.Group{ID: "general", Title: "GENERAL COMMANDS"}, cmdutil.Group{ID: "targeted", Title: "TARGETED COMMANDS"})
	c.AddCommand(newCreate(f), newList(f), newStatus(f),
		newCheckout(f), newChecks(f), newClose(f), newComment(f), newDiff(f), newEdit(f), newMerge(f),
		newReady(f), newReopen(f), newReview(f), newView(f))
	return c
}

const argUse = "[<number> | <url> | <branch>]"

// selectorArgs accepts an optional pull request selector.
var selectorArgs = cmdutil.MaxArgs(1)

// finder resolves the pull request of a command: a number, a URL, a
// branch name, or the pull request of the current branch.
type finder struct {
	f      *cmdutil.Factory
	repo   ghrepo.Repo
	client *api.Client
}

func find(f *cmdutil.Factory, repoFlag string, args []string) (*finder, *shared.PullRequest, error) {
	sel := ""
	if len(args) > 0 {
		sel = args[0]
	}
	var r ghrepo.Repo
	num := 0
	if sel != "" {
		if n, fromURL, err := shared.ParseNumberArg(sel); err == nil {
			num = n
			if fromURL != nil {
				r = *fromURL
				if cfg, err := f.Config(); err == nil {
					if h := cmdutil.MatchHost(cfg.Hosts(), r.Host); h != "" {
						r.Host = h
					}
				}
			}
		}
	}
	if r.Name == "" {
		var err error
		if r, err = f.BaseRepo(repoFlag); err != nil {
			return nil, nil, err
		}
	}
	client, err := f.Client(r.Host)
	if err != nil {
		return nil, nil, err
	}
	fd := &finder{f: f, repo: r, client: client}
	if num > 0 {
		p, err := shared.FetchPull(client, r, num)
		return fd, p, err
	}
	if sel == "" {
		if repoFlag != "" {
			return nil, nil, cmdutil.FlagErrorf("argument required when using the --repo flag")
		}
		if n := checkedOutPull(f); n > 0 {
			p, err := shared.FetchPull(client, r, n)
			return fd, p, err
		}
		branch, err := f.Git("").CurrentBranch()
		if err != nil {
			return nil, nil, err
		}
		sel = branch
	}
	owner, branch, ok := strings.Cut(sel, ":")
	if !ok {
		owner, branch = headOwner(f, r, sel), sel
	}
	p, err := pullForBranch(client, r, owner, branch)
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		return nil, nil, fmt.Errorf("no pull requests found for branch %q", branch)
	}
	return fd, p, nil
}

// checkedOutPull reads branch.<current>.merge = refs/pull/<n>/head, set
// by `eg pr checkout` for pull requests from forks.
func checkedOutPull(f *cmdutil.Factory) int {
	g := f.Git("")
	branch, err := g.CurrentBranch()
	if err != nil {
		return 0
	}
	merge := g.ConfigGet("branch." + branch + ".merge")
	var n int
	if _, err := fmt.Sscanf(merge, "refs/pull/%d/head", &n); err == nil {
		return n
	}
	return 0
}

// headOwner guesses the owner of a local branch from its push remote.
func headOwner(f *cmdutil.Factory, base ghrepo.Repo, branch string) string {
	g := f.Git("")
	remote := g.ConfigGet("branch." + branch + ".pushRemote")
	if remote == "" {
		remote = g.ConfigGet("remote.pushDefault")
	}
	if remote == "" {
		remote = g.ConfigGet("branch." + branch + ".remote")
	}
	if remote != "" {
		if u := g.ConfigGet("remote." + remote + ".url"); u != "" {
			if cfg, err := f.Config(); err == nil {
				if hr, ok := cmdutil.RemoteRepo(cfg.Hosts(), u); ok {
					return hr.Owner
				}
			}
		}
	}
	return base.Owner
}

// pullForBranch returns the open pull request of owner:branch, else the
// most recent one.
func pullForBranch(c *api.Client, r ghrepo.Repo, owner, branch string) (*shared.PullRequest, error) {
	for _, state := range []string{"open", "all"} {
		q := url.Values{"head": {owner + ":" + branch}, "state": {state}, "per_page": {"10"}}
		var list []*shared.PullRequest
		if _, err := c.REST("GET", shared.RepoPath(r)+"/pulls?"+q.Encode(), nil, &list); err != nil {
			return nil, err
		}
		for _, p := range list {
			if p.Head.Ref == branch {
				return p, nil
			}
		}
	}
	return nil, nil
}

// prRef is "owner/repo#N (title)".
func prRef(r ghrepo.Repo, p *shared.PullRequest) string {
	return fmt.Sprintf("%s#%d (%s)", r.FullName(), p.Number, p.Title)
}

// baseRemote finds the git remote that points to r.
func baseRemote(f *cmdutil.Factory, r ghrepo.Repo) (git.Remote, error) {
	remotes, err := f.Git("").Remotes()
	if err != nil {
		return git.Remote{}, err
	}
	cfg, err := f.Config()
	if err != nil {
		return git.Remote{}, err
	}
	for _, rm := range remotes {
		if rr, ok := cmdutil.RemoteRepo(cfg.Hosts(), rm.FetchURL); ok && strings.EqualFold(rr.FullName(), r.FullName()) {
			return rm, nil
		}
	}
	return git.Remote{}, errors.New("no git remote points to " + r.FullName())
}
