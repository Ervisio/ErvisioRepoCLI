package repo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/git"
)

func newSync(f *cmdutil.Factory) *cmdutil.Command {
	var branch, source string
	var force bool
	c := &cmdutil.Command{
		Use:   "sync [<destination-repository>]",
		Short: "Sync a repository",
		Long: `Sync destination repository from source repository. Syncing uses the
default branch of the source repository to update the matching branch on
the destination repository so they are equal. A fast forward update will be
used except when the --force flag is specified, then the two branches will
be synced using a hard reset.

Without an argument, the local repository is selected as the destination
repository and its remote (or --source) as the source.

Syncing a remote repository (passing a destination argument) needs the
merge-upstream API, which the ErvisioRepo server does not offer yet.`,
		Example: `# Sync local repository from remote parent
$ eg repo sync

# Sync local repository from remote parent on specific branch
$ eg repo sync --branch v1`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("sync"),
		Args:    cmdutil.MaxArgs(1),
	}
	c.Flags.StringVarP(&source, "source", "s", "", "Source repository")
	c.Flags.StringVarP(&branch, "branch", "b", "", "Branch to sync (default [default branch])")
	c.Flags.BoolVarP(&force, "force", "", false, "Hard reset the branch of the destination repository to match the source repository")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if len(args) > 0 {
			return cmdutil.Unsupported("Syncing a remote repository", "POST /repos/{owner}/{repo}/merge-upstream")
		}
		return syncLocal(f, source, branch, force)
	}
	return c
}

func syncLocal(f *cmdutil.Factory, source, branch string, force bool) error {
	ios := f.IO
	cfg, err := f.Config()
	if err != nil {
		return err
	}
	var src ghrepo.Repo
	if source != "" {
		if src, err = resolveArg(f, source); err != nil {
			return err
		}
	} else if src, err = f.BaseRepo(""); err != nil {
		return err
	}
	g := f.Git("")
	remotes, err := g.Remotes()
	if err != nil {
		return err
	}
	remote := ""
	for _, rm := range remotes {
		if r, ok := cmdutil.RemoteRepo(cfg.Hosts(), rm.FetchURL); ok && r.Equal(src) {
			remote = rm.Name
			break
		}
	}
	if remote == "" {
		return fmt.Errorf("can't find corresponding remote for %s", src.FullName())
	}
	if branch == "" {
		client, err := f.Client(src.Host)
		if err != nil {
			return err
		}
		repo, err := fetchRepo(client, src)
		if err != nil {
			return err
		}
		branch = repo.DefaultBranch
	}
	if _, err := g.Output("fetch", remote, "+refs/heads/"+branch+":refs/remotes/"+remote+"/"+branch); err != nil {
		return err
	}
	remoteRef := "refs/remotes/" + remote + "/" + branch
	localRef := "refs/heads/" + branch
	hasLocal := true
	if _, err := g.Output("rev-parse", "--verify", "--quiet", localRef); err != nil {
		var ge *git.Error
		if !errors.As(err, &ge) {
			return err
		}
		hasLocal = false
	}
	if hasLocal && !force {
		if _, err := g.Output("merge-base", "--is-ancestor", localRef, remoteRef); err != nil {
			return errors.New("can't sync because there are diverging changes; use `--force` to overwrite the destination branch")
		}
	}
	current, _ := g.CurrentBranch()
	switch {
	case current == branch:
		if out, _ := g.Output("status", "--untracked-files=no", "--porcelain"); strings.TrimSpace(out) != "" {
			return errors.New("can't sync because there are local changes; please stash them before trying again")
		}
		if force {
			_, err = g.Output("reset", "--hard", remoteRef)
		} else {
			_, err = g.Output("merge", "--ff-only", remoteRef)
		}
	case hasLocal:
		_, err = g.Output("update-ref", localRef, remoteRef)
	default:
		_, err = g.Output("branch", branch, "--track", remote+"/"+branch)
	}
	if err != nil {
		return err
	}
	if ios.IsStdoutTTY() {
		fmt.Fprintf(ios.Out, "%s Synced the \"%s\" branch from \"%s\" to local repository\n",
			ios.ColorScheme().SuccessIcon(), branch, src.FullName())
	}
	return nil
}
