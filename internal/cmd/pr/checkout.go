package pr

import (
	"fmt"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

func newCheckout(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, branch                string
		force, detach, recurseSubmodule bool
	)
	c := &cmdutil.Command{
		Use:     "checkout {<number> | <url> | <branch>}",
		Short:   "Check out a pull request in git",
		Aliases: []string{"co"},
		GroupID: "targeted",
		Example: `# Checkout a specific pull request
$ eg pr checkout 32

# Checkout a pull request using its URL
$ eg pr checkout https://code.example.com/OWNER/REPO/pulls/32

# Checkout a pull request using its head branch
$ eg pr checkout feature`,
		Flags: cmdutil.NewFlagSet("checkout"),
		Args:  cmdutil.ExactArgs(1, "argument required: pull request number, URL or branch"),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.StringVarP(&branch, "branch", "b", "", "Local branch name to use (default [the name of the head branch])")
	fl.BoolVarP(&detach, "detach", "", false, "Checkout PR with a detached HEAD")
	fl.BoolVarP(&force, "force", "f", false, "Reset the existing local branch to the latest state of the pull request")
	fl.BoolVarP(&recurseSubmodule, "recurse-submodules", "", false, "Update all submodules after checkout")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		g := f.Git("")
		remoteName := ""
		if rm, err := baseRemote(f, fd.repo); err == nil {
			remoteName = rm.Name
		}
		fetchFrom := remoteName
		if fetchFrom == "" {
			cfg, err := f.Config()
			if err != nil {
				return err
			}
			fetchFrom = ghrepo.CloneURL(fd.repo, cfg.GitProtocol(fd.repo.Host), cfg.Scheme(fd.repo.Host))
		}
		local := branch
		if local == "" {
			local = p.Head.Ref
			if p.IsCrossRepository() && p.Head.Repo != nil && (local == "main" || local == "master" || local == p.Base.Ref) {
				local = p.Head.Repo.Owner.Login + "/" + local
			}
		}
		pullRef := fmt.Sprintf("refs/pull/%d/head", p.Number)
		var cmds [][]string
		switch {
		case detach:
			cmds = [][]string{{"fetch", fetchFrom, pullRef}, {"checkout", "--detach", "FETCH_HEAD"}}
		case !p.IsCrossRepository() && remoteName != "" && p.Head.Repo != nil:
			tracking := "refs/remotes/" + remoteName + "/" + p.Head.Ref
			cmds = [][]string{{"fetch", remoteName, "+refs/heads/" + p.Head.Ref + ":" + tracking}}
			if hasLocalBranch(f, local) {
				cmds = append(cmds, []string{"checkout", local})
				if force {
					cmds = append(cmds, []string{"reset", "--hard", tracking})
				} else {
					cmds = append(cmds, []string{"merge", "--ff-only", tracking})
				}
			} else {
				cmds = append(cmds, []string{"checkout", "-b", local, "--track", remoteName + "/" + p.Head.Ref})
			}
		default:
			// A fork (or a deleted head repository): the server keeps
			// refs/pull/<n>/head in the base repository.
			current, _ := g.CurrentBranch()
			if local == current {
				spec := pullRef
				if force {
					cmds = [][]string{{"fetch", fetchFrom, spec}, {"reset", "--hard", "FETCH_HEAD"}}
				} else {
					cmds = [][]string{{"fetch", fetchFrom, spec}, {"merge", "--ff-only", "FETCH_HEAD"}}
				}
			} else {
				spec := pullRef + ":" + local
				if force {
					spec = "+" + spec
				}
				cmds = [][]string{{"fetch", fetchFrom, spec}, {"checkout", local}}
			}
			remote := remoteName
			if remote == "" {
				remote = fetchFrom
			}
			cmds = append(cmds,
				[]string{"config", "branch." + local + ".remote", remote},
				[]string{"config", "branch." + local + ".merge", pullRef})
		}
		if recurseSubmodule {
			cmds = append(cmds, []string{"submodule", "sync", "--recursive"}, []string{"submodule", "update", "--init", "--recursive"})
		}
		for _, a := range cmds {
			if err := g.Run(a...); err != nil {
				return err
			}
		}
		return nil
	}
	return c
}

func hasLocalBranch(f *cmdutil.Factory, b string) bool {
	_, err := f.Git("").Output("rev-parse", "--verify", "--quiet", "refs/heads/"+strings.TrimPrefix(b, "refs/heads/"))
	return err == nil
}
