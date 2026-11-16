package repo

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/git"
)

func newFork(f *cmdutil.Factory) *cmdutil.Command {
	var (
		clone, remote, defaultBranchOnly bool
		remoteName, org, forkName        string
	)
	c := &cmdutil.Command{
		Use:   "fork [<repository>] [-- <gitflags>...]",
		Short: "Create a fork of a repository",
		Long: `Create a fork of a repository.

With no argument, creates a fork of the current repository. Otherwise,
forks the specified repository.

By default, the new fork is set to be your "origin" remote and any existing
origin remote is renamed to "upstream". To alter this behavior, you can set
a name for the new fork's remote with --remote-name.

The "upstream" remote will be set as the default remote repository.

Additional git clone flags can be passed after "--".

The server copies every branch: --default-branch-only is accepted for
compatibility but has no effect.`,
		GroupID:  "targeted",
		Flags:    cmdutil.NewFlagSet("fork"),
		KeepDash: true,
		Args:     cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	fl.BoolVarP(&clone, "clone", "", false, "Clone the fork")
	fl.BoolVarP(&remote, "remote", "", false, "Add a git remote for the fork")
	fl.StringVarP(&remoteName, "remote-name", "", "origin", "Specify the name for the new remote")
	fl.StringVarP(&org, "org", "", "", "Create the fork in an organization")
	fl.StringVarP(&forkName, "fork-name", "", "", "Rename the forked repository")
	fl.BoolVarP(&defaultBranchOnly, "default-branch-only", "", false, "Only include the default branch in the fork")
	c.Run = func(c *cmdutil.Command, args []string) error {
		if len(c.DashArgs) > 0 && len(args) == 0 {
			return cmdutil.FlagErrorf("repository argument required when passing git clone flags")
		}
		ios := f.IO
		cs := ios.ColorScheme()
		inParent := len(args) == 0
		var (
			src ghrepo.Repo
			err error
		)
		if inParent {
			src, err = f.BaseRepo("")
		} else {
			src, err = resolveArg(f, args[0])
		}
		if err != nil {
			return err
		}
		client, err := f.Client(src.Host)
		if err != nil {
			return err
		}
		body := map[string]any{}
		if org != "" {
			body["organization"] = org
		}
		if forkName != "" {
			body["name"] = forkName
		}
		if defaultBranchOnly {
			body["default_branch_only"] = true
		}
		var fork Repository
		if _, err := client.REST(http.MethodPost, repoPath(src)+"/forks", body, &fork); err != nil {
			return fmt.Errorf("failed to fork: %w", err)
		}
		forkRef := fork.Ref(src.Host)
		created, _ := time.Parse(time.RFC3339, fork.CreatedAt)
		existed := !created.IsZero() && time.Since(created) > time.Minute
		switch {
		case existed && ios.IsStderrTTY():
			fmt.Fprintf(ios.ErrOut, "%s %s already exists\n", cs.Yellow("!"), cs.Bold(forkRef.FullName()))
		case existed:
			fmt.Fprintf(ios.ErrOut, "%s already exists\n", forkRef.FullName())
		case ios.IsStderrTTY():
			fmt.Fprintf(ios.ErrOut, "%s Created fork %s\n", cs.SuccessIcon(), cs.Bold(forkRef.FullName()))
		}
		if forkName != "" && !strings.EqualFold(fork.Name, forkName) {
			// An existing fork keeps its name: rename it as gh does.
			var renamed Repository
			if _, err := client.REST(http.MethodPatch, repoPath(forkRef), map[string]any{"name": forkName}, &renamed); err != nil {
				return fmt.Errorf("could not rename fork: %w", err)
			}
			fork, forkRef = renamed, renamed.Ref(src.Host)
			if ios.IsStderrTTY() {
				fmt.Fprintf(ios.ErrOut, "%s Renamed fork to %s\n", cs.SuccessIcon(), cs.Bold(forkRef.FullName()))
			}
		}
		prompt := ios.CanPrompt()
		p := &cmdutil.Prompter{IO: ios}
		if inParent {
			if !remote && !c.Flags.Changed("remote") && prompt {
				if remote, err = p.Confirm("Would you like to add a remote for the fork?", true); err != nil {
					return err
				}
			}
			if !remote {
				return nil
			}
			return addForkRemote(f, forkRef, remoteName, !c.Flags.Changed("remote-name"))
		}
		if !clone && !c.Flags.Changed("clone") && prompt {
			if clone, err = p.Confirm("Would you like to clone the fork?", true); err != nil {
				return err
			}
		}
		if !clone {
			return nil
		}
		gitArgs := append([]string{"clone"}, c.DashArgs...)
		gitArgs = append(gitArgs, "--", cloneURL(f, forkRef))
		if err := f.Git("").Run(gitArgs...); err != nil {
			return err
		}
		dir := path.Base(forkRef.Name)
		parent := fork.Parent
		if parent == nil {
			if parent, err = fetchRepo(client, src); err != nil {
				return err
			}
		}
		if err := addUpstream(f, dir, parent, src.Host, "upstream"); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Cloned fork\n", cs.SuccessIcon())
		}
		return nil
	}
	return c
}

// addForkRemote adds the fork as remote name of the current clone. When
// rename is set and the name is taken, the existing remote becomes
// "upstream" as in gh.
func addForkRemote(f *cmdutil.Factory, fork ghrepo.Repo, name string, rename bool) error {
	ios := f.IO
	cs := ios.ColorScheme()
	g := f.Git("")
	remotes, err := g.Remotes()
	if err != nil {
		if errors.Is(err, git.ErrNotRepo) {
			return errors.New("not a git repository: cannot add a remote for the fork")
		}
		return err
	}
	url := cloneURL(f, fork)
	for _, rm := range remotes {
		if rm.Name != name {
			continue
		}
		if !rename {
			return fmt.Errorf("a git remote named '%s' already exists", name)
		}
		for _, other := range remotes {
			if other.Name == "upstream" {
				return fmt.Errorf("a git remote named '%s' already exists and 'upstream' is taken", name)
			}
		}
		if _, err := g.Output("remote", "rename", name, "upstream"); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Renamed remote %s to %s\n", cs.SuccessIcon(), cs.Bold(name), cs.Bold("upstream"))
		}
		break
	}
	if _, err := g.Output("remote", "add", name, url); err != nil {
		return err
	}
	if _, err := g.Output("fetch", name); err != nil {
		return err
	}
	if ios.IsStderrTTY() {
		fmt.Fprintf(ios.ErrOut, "%s Added remote %s\n", cs.SuccessIcon(), cs.Bold(name))
	}
	return nil
}
