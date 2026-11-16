package repo

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/git"
)

type createOptions struct {
	description                 string
	public, private, internal   bool
	clone, push                 bool
	source, remote              string
	template, gitignore, licens string
	addReadme                   bool
}

func newCreate(f *cmdutil.Factory) *cmdutil.Command {
	o := &createOptions{}
	c := &cmdutil.Command{
		Use:   "create [<name>]",
		Short: "Create a new repository",
		Long: `Create a new ErvisioRepo repository.

To create a repository interactively, use eg repo create with no arguments.

To create a remote repository non-interactively, supply the repository name
and one of --public, --private, or --internal. Pass --clone to clone the
new repository locally.

To create a remote repository from an existing local repository, specify
the source directory with --source. By default, the remote repository name
will be the name of the source directory. Pass --push to push any local
commits to the new repository.`,
		Example: `# create a repository interactively
$ eg repo create

# create a new remote repository and clone it locally
$ eg repo create my-project --public --clone

# create a new remote repository in a different organization
$ eg repo create my-org/my-project --public

# create a remote repository from the current directory
$ eg repo create my-project --private --source=. --remote=upstream`,
		Aliases: []string{"new"},
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("create"),
		Args:    cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	fl.StringVarP(&o.description, "description", "d", "", "Description of the repository")
	fl.BoolVarP(&o.public, "public", "", false, "Make the new repository public")
	fl.BoolVarP(&o.private, "private", "", false, "Make the new repository private")
	fl.BoolVarP(&o.internal, "internal", "", false, "Make the new repository internal")
	fl.BoolVarP(&o.clone, "clone", "c", false, "Clone the new repository to the current directory")
	fl.StringVarP(&o.source, "source", "s", "", "Specify `path` to local repository to use as source")
	fl.StringVarP(&o.remote, "remote", "r", "", "Specify remote name for the new repository")
	fl.BoolVarP(&o.push, "push", "", false, "Push local commits to the new repository")
	fl.StringVarP(&o.template, "template", "p", "", "Make the new repository based on a template `repository` (not supported yet)")
	fl.StringVarP(&o.gitignore, "gitignore", "g", "", "Specify a gitignore template for the repository")
	fl.StringVarP(&o.licens, "license", "l", "", "Specify an Open Source License for the repository")
	fl.BoolVarP(&o.addReadme, "add-readme", "", false, "Add a README file to the new repository")
	c.Run = func(c *cmdutil.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		}
		return runCreate(f, c, o, name)
	}
	return c
}

func runCreate(f *cmdutil.Factory, c *cmdutil.Command, o *createOptions, name string) error {
	switch {
	case o.template != "":
		return cmdutil.Unsupported("Creating a repository from a template", "template support in POST /user/repos")
	case o.source != "" && (o.gitignore != "" || o.licens != "" || o.addReadme):
		return cmdutil.FlagErrorf("the `--add-readme`, `--gitignore` and `--license` options are not supported with `--source`")
	}
	if o.source != "" && o.clone {
		return cmdutil.FlagErrorf("the `--source` option is not supported with `--clone`")
	}
	if o.source == "" && (o.push || c.Flags.Changed("remote")) {
		return cmdutil.FlagErrorf("the `--push` and `--remote` options require `--source`")
	}
	n := 0
	for _, b := range []bool{o.public, o.private, o.internal} {
		if b {
			n++
		}
	}
	if n > 1 {
		return cmdutil.FlagErrorf("expected exactly one of `--public`, `--private`, or `--internal`")
	}
	ios := f.IO
	cs := ios.ColorScheme()
	interactive := name == "" && o.source == "" && n == 0
	if interactive && !ios.CanPrompt() {
		return cmdutil.FlagErrorf("at least one argument required in non-interactive mode")
	}
	if name == "" && o.source != "" {
		abs, err := filepath.Abs(o.source)
		if err != nil {
			return fmt.Errorf("resolve source path: %w", err)
		}
		name = filepath.Base(abs)
	}
	visibility := ""
	switch {
	case o.public:
		visibility = "public"
	case o.private:
		visibility = "private"
	case o.internal:
		visibility = "internal"
	}
	if interactive {
		p := &cmdutil.Prompter{IO: ios}
		var err error
		if name, err = p.Input("Repository name", ""); err != nil {
			return err
		}
		if name == "" {
			return errors.New("a repository name is required")
		}
		if o.description, err = p.Input("Description", ""); err != nil {
			return err
		}
		opts := []string{"Public", "Private", "Internal"}
		i, err := p.Select("Visibility", 0, opts)
		if err != nil {
			return err
		}
		visibility = strings.ToLower(opts[i])
		if err := promptInit(f, p, o); err != nil {
			return err
		}
		if o.clone, err = p.Confirm("Clone the new repository locally?", true); err != nil {
			return err
		}
	}
	if visibility == "" {
		return cmdutil.FlagErrorf("`--public`, `--private`, or `--internal` required when not running interactively")
	}

	host, err := f.DefaultHost()
	if err != nil {
		return err
	}
	owner, repoName, hasOwner := strings.Cut(name, "/")
	if !hasOwner {
		owner, repoName = "", name
	} else if strings.Contains(repoName, "/") {
		r, err := ghrepo.FromFullName(name, host)
		if err != nil {
			return err
		}
		host, owner, repoName = r.Host, r.Owner, r.Name
	}
	client, err := f.Client(host)
	if err != nil {
		return err
	}
	endpoint := "user/repos"
	if owner != "" {
		login, err := viewerLogin(client)
		if err != nil {
			return err
		}
		if !strings.EqualFold(login, owner) {
			endpoint = "orgs/" + url.PathEscape(owner) + "/repos"
		}
	}
	body := map[string]any{"name": repoName, "visibility": visibility, "private": visibility != "public"}
	if o.description != "" {
		body["description"] = o.description
	}
	if o.addReadme {
		body["auto_init"] = true
	}
	if o.gitignore != "" {
		body["gitignore_template"] = o.gitignore
	}
	if o.licens != "" {
		body["license_template"] = o.licens
	}
	initialized := o.addReadme || o.gitignore != "" || o.licens != ""
	var created Repository
	if _, err := client.REST(http.MethodPost, endpoint, body, &created); err != nil {
		return err
	}
	ref := created.Ref(host)
	if ios.IsStdoutTTY() {
		fmt.Fprintf(ios.Out, "%s Created repository %s on ErvisioRepo\n  %s\n", cs.SuccessIcon(), created.FullName, created.HTMLURL)
	} else {
		fmt.Fprintln(ios.Out, created.HTMLURL)
	}
	remoteURL := cloneURL(f, ref)

	if o.source != "" {
		return pushSource(f, o, remoteURL)
	}
	if o.clone && initialized {
		return f.Git("").Run("clone", "--", remoteURL)
	}
	if o.clone {
		// The repository is empty: initialize it locally as gh does.
		g := f.Git("")
		if _, err := g.Output("init", "--", ref.Name); err != nil {
			return err
		}
		if _, err := f.Git(ref.Name).Output("remote", "add", "origin", remoteURL); err != nil {
			return err
		}
		if ios.IsStdoutTTY() {
			fmt.Fprintf(ios.Out, "%s Initialized repository in \"%s\"\n", cs.SuccessIcon(), ref.Name)
		}
	}
	return nil
}

func pushSource(f *cmdutil.Factory, o *createOptions, remoteURL string) error {
	ios := f.IO
	cs := ios.ColorScheme()
	g := f.Git(o.source)
	if _, err := g.GitDir(); err != nil {
		if errors.Is(err, git.ErrNotRepo) {
			return fmt.Errorf("current directory is not a git repository. Run `git -C %q init` to initialize it", o.source)
		}
		return err
	}
	remote := o.remote
	if remote == "" {
		remote = "origin"
	}
	if _, err := g.Output("remote", "add", remote, remoteURL); err != nil {
		return fmt.Errorf("unable to add remote %q: %w", remote, err)
	}
	if ios.IsStdoutTTY() {
		fmt.Fprintf(ios.Out, "%s Added remote %s\n", cs.SuccessIcon(), remoteURL)
	}
	if !o.push {
		return nil
	}
	if err := g.Run("push", "--set-upstream", remote, "HEAD"); err != nil {
		return err
	}
	if ios.IsStdoutTTY() {
		fmt.Fprintf(ios.Out, "%s Pushed commits to %s\n", cs.SuccessIcon(), remoteURL)
	}
	return nil
}

// promptInit asks for the README, .gitignore and license of a new
// repository, as gh does.
func promptInit(f *cmdutil.Factory, p *cmdutil.Prompter, o *createOptions) error {
	var err error
	if o.addReadme, err = p.Confirm("Would you like to add a README file?", false); err != nil {
		return err
	}
	host, err := f.DefaultHost()
	if err != nil {
		return err
	}
	client, err := f.Client(host)
	if err != nil {
		return err
	}
	if ok, err := p.Confirm("Would you like to add a .gitignore?", false); err != nil {
		return err
	} else if ok {
		names, err := gitignoreNames(client)
		if err != nil {
			return err
		}
		i, err := p.Select("Choose a .gitignore template", 0, names)
		if err != nil {
			return err
		}
		o.gitignore = names[i]
	}
	if ok, err := p.Confirm("Would you like to add a license?", false); err != nil {
		return err
	} else if ok {
		lics, err := listLicenses(client)
		if err != nil {
			return err
		}
		opts := make([]string, len(lics))
		for i, l := range lics {
			opts[i] = l.Name
		}
		i, err := p.Select("Choose a license", 0, opts)
		if err != nil {
			return err
		}
		o.licens = lics[i].Key
	}
	return nil
}
