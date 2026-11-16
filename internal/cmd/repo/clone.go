package repo

import (
	"path"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

func newClone(f *cmdutil.Factory) *cmdutil.Command {
	var upstreamName string
	var noUpstream bool
	c := &cmdutil.Command{
		Use:   "clone <repository> [<directory>] [-- <gitflags>...]",
		Short: "Clone a repository locally",
		Long: `Clone an ErvisioRepo repository locally. Pass additional git clone flags
by listing them after "--".

If the "OWNER/" portion of the "OWNER/REPO" repository argument is omitted,
it defaults to the name of the authenticating user.

When a protocol scheme is not provided in the repository argument, the
git_protocol of the host is used (see eg auth login --git-protocol).

If the repository is a fork, its parent repository will be added as an
additional git remote called "upstream". The remote name can be configured
using --upstream-remote-name. The --upstream-remote-name option supports an
"@owner" value which will name the remote after the owner of the parent
repository.`,
		Example: `# Clone a repository from a specific org
$ eg repo clone acme/widgets

# Clone a repository from a different host
$ eg repo clone code.example.com/acme/widgets

# Clone a repository using a URL
$ eg repo clone https://code.example.com/acme/widgets

# Clone a repository with additional git clone flags
$ eg repo clone acme/widgets workspace/widgets -- --depth=1`,
		GroupID:  "general",
		Flags:    cmdutil.NewFlagSet("clone"),
		KeepDash: true,
		Args:     cmdutil.RangeArgs(1, 2, "cannot clone: repository argument required"),
	}
	c.Flags.StringVarP(&upstreamName, "upstream-remote-name", "u", "upstream", "Upstream remote name when cloning a fork")
	c.Flags.BoolVarP(&noUpstream, "no-upstream", "", false, "Do not add an upstream remote when cloning a fork")
	c.Run = func(c *cmdutil.Command, args []string) error {
		if noUpstream && c.Flags.Changed("upstream-remote-name") {
			return cmdutil.FlagErrorf("`--no-upstream` cannot be used with `--upstream-remote-name`")
		}
		arg := args[0]
		r, err := resolveArg(f, arg)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		repo, err := fetchRepo(client, r)
		if err != nil {
			return err
		}
		canonical := repo.Ref(r.Host)
		url := cloneURL(f, canonical)
		if ghrepo.IsURL(arg) {
			// Keep the protocol the user asked for.
			url = arg
			if strings.HasPrefix(arg, "http") && !strings.HasSuffix(url, ".git") {
				url = strings.TrimSuffix(url, "/") + ".git"
			}
		}
		dir := ""
		if len(args) > 1 {
			dir = args[1]
		}
		gitArgs := append([]string{"clone"}, c.DashArgs...)
		gitArgs = append(gitArgs, "--", url)
		if dir != "" {
			gitArgs = append(gitArgs, dir)
		} else {
			dir = path.Base(strings.TrimSuffix(canonical.Name, ".git"))
		}
		g := f.Git("")
		if err := g.Run(gitArgs...); err != nil {
			return err
		}
		if repo.Parent == nil || noUpstream {
			return nil
		}
		parent := repo.Parent.Ref(r.Host)
		name := upstreamName
		if name == "@owner" {
			name = parent.Owner
		}
		inRepo := f.Git(dir)
		if repo.Parent.DefaultBranch != "" {
			err = inRepo.Run("remote", "add", "-t", repo.Parent.DefaultBranch, name, cloneURL(f, parent))
		} else {
			err = inRepo.Run("remote", "add", name, cloneURL(f, parent))
		}
		if err != nil {
			return err
		}
		if err := inRepo.Run("fetch", name); err != nil {
			return err
		}
		_, err = inRepo.Output("config", "--add", "remote."+name+".eg-resolved", "base")
		return err
	}
	return c
}
