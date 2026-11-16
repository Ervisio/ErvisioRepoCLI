package repo

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// targetRepo resolves the optional repository argument or --repo.
func targetRepo(f *cmdutil.Factory, args []string, override string) (ghrepo.Repo, error) {
	if len(args) > 0 {
		return resolveArg(f, args[0])
	}
	return f.BaseRepo(override)
}

func newDelete(f *cmdutil.Factory) *cmdutil.Command {
	var yes bool
	c := &cmdutil.Command{
		Use:   "delete [<repository>]",
		Short: "Delete a repository",
		Long: `Delete an ErvisioRepo repository.

With no argument, deletes the current repository. Otherwise, deletes the
specified repository.

Deletion requires authorization with a token that can administer the
repository.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("delete"),
		Args:    cmdutil.MaxArgs(1),
	}
	c.Flags.BoolVarP(&yes, "yes", "", false, "Confirm deletion without prompting")
	c.Flags.BoolVarP(&yes, "confirm", "", false, "Deprecated: use --yes instead")
	c.Flags.Hide("confirm")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		ios := f.IO
		if !yes && !ios.CanPrompt() {
			return cmdutil.FlagErrorf("--yes required when not running interactively")
		}
		r, err := targetRepo(f, args, "")
		if err != nil {
			return err
		}
		if !yes {
			p := &cmdutil.Prompter{IO: ios}
			ans, err := p.Input(fmt.Sprintf("Type %s to confirm deletion:", r.FullName()), "")
			if err != nil {
				return err
			}
			if !strings.EqualFold(ans, r.FullName()) {
				return cmdutil.ErrCancel
			}
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if _, err := client.REST(http.MethodDelete, repoPath(r), nil, nil); err != nil {
			return err
		}
		if ios.IsStdoutTTY() {
			fmt.Fprintf(ios.Out, "%s Deleted repository %s\n", ios.ColorScheme().SuccessIcon(), r.FullName())
		}
		return nil
	}
	return c
}

// unsupportedEditFlags are gh repo edit flags the server cannot apply yet.
var unsupportedEditFlags = []string{"homepage", "add-topic", "remove-topic", "enable-issues", "enable-wiki",
	"enable-projects", "enable-discussions", "enable-merge-commit", "enable-squash-merge",
	"enable-rebase-merge", "enable-auto-merge", "delete-branch-on-merge", "template", "allow-forking"}

func newEdit(f *cmdutil.Factory) *cmdutil.Command {
	var (
		description, defaultBranch, visibility string
		acceptVisibility                       bool
		unsupported                            string
	)
	c := &cmdutil.Command{
		Use:   "edit [<repository>]",
		Short: "Edit repository settings",
		Long: `Edit repository settings.

To toggle a setting off, use the --<flag>=false syntax.

Changing repository visibility can have unexpected consequences including
but not limited to losing access for some users. Pass
--accept-visibility-change-consequences to confirm you understand them.

The ErvisioRepo server supports editing the description, the default branch
and the visibility; other gh settings (homepage, topics, merge options,
archiving) are not supported yet.`,
		Example: `# enable the description and set the default branch
$ eg repo edit --description "My project" --default-branch main

# change the visibility
$ eg repo edit --visibility private --accept-visibility-change-consequences`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("edit"),
		Args:    cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	fl.StringVarP(&description, "description", "d", "", "Description of the repository")
	fl.StringVarP(&defaultBranch, "default-branch", "", "", "Set the default branch `name` for the repository")
	fl.EnumVarP(&visibility, "visibility", "", "", []string{"public", "private", "internal"}, "Change the visibility of the repository")
	fl.BoolVarP(&acceptVisibility, "accept-visibility-change-consequences", "", false, "Accept the consequences of changing the repository visibility")
	for _, n := range unsupportedEditFlags {
		fl.StringVarP(&unsupported, n, "", "", "")
		fl.Hide(n)
	}
	c.Run = func(c *cmdutil.Command, args []string) error {
		for _, n := range unsupportedEditFlags {
			if c.Flags.Changed(n) {
				return cmdutil.Unsupported("--"+n, "the matching field in PATCH /repos/{owner}/{repo}")
			}
		}
		body := map[string]any{}
		if c.Flags.Changed("description") {
			body["description"] = description
		}
		if c.Flags.Changed("default-branch") {
			body["default_branch"] = defaultBranch
		}
		if c.Flags.Changed("visibility") {
			if !acceptVisibility {
				return cmdutil.FlagErrorf("use of --visibility flag requires --accept-visibility-change-consequences flag")
			}
			body["visibility"] = visibility
		}
		if len(body) == 0 {
			return cmdutil.FlagErrorf("specify properties to edit when not running interactively")
		}
		r, err := targetRepo(f, args, "")
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if _, err := client.REST(http.MethodPatch, repoPath(r), body, nil); err != nil {
			return err
		}
		if f.IO.IsStdoutTTY() {
			fmt.Fprintf(f.IO.Out, "%s Edited repository %s\n", f.IO.ColorScheme().SuccessIcon(), r.FullName())
		}
		return nil
	}
	return c
}

func newRename(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	var yes bool
	c := &cmdutil.Command{
		Use:   "rename [<new-name>]",
		Short: "Rename a repository",
		Long: `Rename an ErvisioRepo repository.

By default, this renames the current repository; otherwise renames the
specified repository with --repo.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("rename"),
		Args:    cmdutil.MaxArgs(1),
	}
	c.Flags.StringVarP(&repoFlag, "repo", "R", "", "Select another repository using the `[HOST/]OWNER/REPO` format")
	c.Flags.BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		ios := f.IO
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		newName := ""
		if len(args) > 0 {
			newName = args[0]
		}
		p := &cmdutil.Prompter{IO: ios}
		if newName == "" {
			if !ios.CanPrompt() {
				return cmdutil.FlagErrorf("new name argument required when not running interactively")
			}
			if newName, err = p.Input(fmt.Sprintf("Rename %s to:", r.FullName()), ""); err != nil {
				return err
			}
		} else if repoFlag == "" && !yes {
			if !ios.CanPrompt() {
				return cmdutil.FlagErrorf("--yes required when passing a single argument")
			}
			ok, err := p.Confirm(fmt.Sprintf("Rename %s to %s?", r.FullName(), newName), false)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.ErrCancel
			}
		}
		if newName == "" || strings.Contains(newName, "/") {
			return errors.New("invalid new name")
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var updated Repository
		if _, err := client.REST(http.MethodPatch, repoPath(r), map[string]any{"name": newName}, &updated); err != nil {
			return err
		}
		cs := ios.ColorScheme()
		if ios.IsStdoutTTY() {
			fmt.Fprintf(ios.Out, "%s Renamed repository %s\n", cs.SuccessIcon(), updated.FullName)
		}
		if repoFlag != "" {
			return nil
		}
		// Update the remote that points to the old name.
		g := f.Git("")
		remotes, err := g.Remotes()
		if err != nil {
			return nil //nolint:nilerr // not in a repository: nothing to update
		}
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		for _, rm := range remotes {
			rr, ok := cmdutil.RemoteRepo(cfg.Hosts(), rm.FetchURL)
			if !ok || !rr.Equal(r) {
				continue
			}
			newURL := cloneURL(f, updated.Ref(r.Host))
			if _, err := g.Output("remote", "set-url", rm.Name, newURL); err != nil {
				return err
			}
			if ios.IsStdoutTTY() {
				fmt.Fprintf(ios.Out, "%s Updated the %q remote\n", cs.SuccessIcon(), rm.Name)
			}
			break
		}
		return nil
	}
	return c
}
