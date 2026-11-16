package release

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// noteFlags are the release notes flags shared by create and edit.
type noteFlags struct {
	notes, notesFile, title, target string
	draft, prerelease, latest       bool
	verifyTag                       bool
}

func (n *noteFlags) register(fl *cmdutil.FlagSet) {
	fl.BoolVarP(&n.draft, "draft", "d", false, "Save the release as a draft instead of publishing it")
	fl.BoolVarP(&n.prerelease, "prerelease", "p", false, "Mark the release as a prerelease")
	fl.BoolVarP(&n.latest, "latest", "", false, "Mark this release as \"Latest\" (not supported: the server picks the latest release)")
	fl.StringVarP(&n.notes, "notes", "n", "", "Release notes")
	fl.StringVarP(&n.notesFile, "notes-file", "F", "", "Read release notes from `file` (use \"-\" to read from standard input)")
	fl.StringVarP(&n.title, "title", "t", "", "Release title")
	fl.StringVarP(&n.target, "target", "", "", "Target `branch` or full commit SHA (default [main branch])")
	fl.BoolVarP(&n.verifyTag, "verify-tag", "", false, "Abort in case the git tag doesn't already exist in the remote repository")
}

func (n *noteFlags) validate(f *cmdutil.Factory, fl *cmdutil.FlagSet) error {
	if fl.Changed("latest") {
		return cmdutil.Unsupported("Choosing the latest release (--latest)", "make_latest in POST/PATCH /repos/{owner}/{repo}/releases")
	}
	if fl.Changed("notes") && n.notesFile != "" {
		return cmdutil.FlagErrorf("specify only one of `--notes` or `--notes-file`")
	}
	if n.notesFile != "" {
		b, err := shared.ReadBodyFile(f.IO, n.notesFile)
		if err != nil {
			return err
		}
		n.notes = b
		fl.Set("notes", b) //nolint:errcheck // known flag
	}
	return nil
}

func tagExists(c *api.Client, r ghrepo.Repo, tag string) (bool, error) {
	_, err := c.REST(http.MethodGet, shared.RepoPath(r)+"/git/ref/tags/"+escapePath(tag), nil, nil)
	if err == nil {
		return true, nil
	}
	if api.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func newCreate(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, notesStartTag, discussion string
		generateNotes, notesFromTag         bool
		n                                   noteFlags
	)
	c := &cmdutil.Command{
		Use:   "create [<tag>] [<files>...]",
		Short: "Create a new release",
		Long: `Create a new ErvisioRepo Release for a repository.

A list of asset files may be given to upload to the new release. To define
a display label for an asset, append text starting with '#' after the file
name.

If a matching git tag does not yet exist, one will automatically get
created from the latest state of the default branch. Use '--target' to
point to a different branch or commit for the automatic tag creation. Use
'--verify-tag' to abort the release if the tag doesn't already exist.

To create a release from an annotated git tag, first create one locally
with git, push the tag to ErvisioRepo, then run this command. Use
'--notes-from-tag' to get the release notes from the annotated git tag.

Automatically generated release notes (--generate-notes) are not supported
by ErvisioRepo yet.`,
		Aliases: []string{"new"},
		GroupID: "general",
		Example: `Interactively create a release
$ eg release create

Non-interactively create a release
$ eg release create v1.2.3 --notes "bugfix release"

Use release notes from a file
$ eg release create v1.2.3 -F changelog.md

Upload all tarballs in a directory as release assets
$ eg release create v1.2.3 ./dist/*.tgz

Upload a release asset with a display label
$ eg release create v1.2.3 '/path/to/asset.zip#My display label'`,
		Flags: cmdutil.NewFlagSet("create"),
	}
	fl := c.Flags
	addRepoFlag(c, &repoFlag)
	n.register(fl)
	fl.BoolVarP(&generateNotes, "generate-notes", "", false, "Automatically generate title and notes for the release (not supported)")
	fl.StringVarP(&notesStartTag, "notes-start-tag", "", "", "Tag to use as the starting point for generating release notes (not supported)")
	fl.BoolVarP(&notesFromTag, "notes-from-tag", "", false, "Automatically generate notes from annotated tag")
	fl.StringVarP(&discussion, "discussion-category", "", "", "Start a discussion in the specified category (not supported)")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if generateNotes || notesStartTag != "" {
			return cmdutil.Unsupported("Generating release notes", "POST /repos/{owner}/{repo}/releases/generate-notes")
		}
		if discussion != "" {
			return cmdutil.Unsupported("Release discussions", "discussions")
		}
		if err := n.validate(f, fl); err != nil {
			return err
		}
		if notesFromTag && fl.Changed("notes") {
			return cmdutil.FlagErrorf("using `--notes-from-tag` with `--notes` or `--notes-file` is not supported")
		}
		ios := f.IO
		tag := ""
		var files []uploadFile
		if len(args) > 0 {
			tag = args[0]
			var err error
			if files, err = parseFiles(args[1:]); err != nil {
				return err
			}
		}
		if tag == "" && !ios.CanPrompt() {
			return cmdutil.FlagErrorf("tag required when not running interactively")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		p := &cmdutil.Prompter{IO: ios}
		if tag == "" {
			if tag, err = p.Input("Tag name", ""); err != nil {
				return err
			}
			if tag == "" {
				return errors.New("tag required")
			}
		}
		exists, err := tagExists(client, r, tag)
		if err != nil {
			return err
		}
		if n.verifyTag && !exists {
			return fmt.Errorf("tag %s doesn't exist in the repo %s, aborting due to --verify-tag flag", tag, r.FullName())
		}
		if notesFromTag {
			out, err := f.Git("").Output("tag", "--list", tag, "--format=%(contents)")
			if err != nil || strings.TrimSpace(out) == "" {
				return fmt.Errorf("cannot generate release notes from tag %s as it does not exist locally or is not annotated", tag)
			}
			n.notes = strings.TrimSpace(out)
			fl.Set("notes", n.notes) //nolint:errcheck // known flag
		}
		if ios.CanPrompt() && !fl.Changed("notes") && !fl.Changed("title") {
			if n.title, err = p.Input("Title (optional)", n.title); err != nil {
				return err
			}
			if n.notes, err = shared.EditorPrompt(f, "Release notes", n.notes); err != nil {
				return err
			}
			if !fl.Changed("prerelease") {
				if n.prerelease, err = p.Confirm("Is this a prerelease?", false); err != nil {
					return err
				}
			}
			if !fl.Changed("draft") {
				i, err := p.Select("Submit?", 0, []string{"Publish release", "Save as draft", "Cancel"})
				if err != nil {
					return err
				}
				switch i {
				case 1:
					n.draft = true
				case 2:
					return cmdutil.ErrCancel
				}
			}
		}
		req := map[string]any{"tag_name": tag, "draft": n.draft || len(files) > 0, "prerelease": n.prerelease}
		if n.title != "" {
			req["name"] = n.title
		}
		if n.notes != "" {
			req["body"] = n.notes
		}
		if n.target != "" {
			req["target_commitish"] = n.target
		}
		var rel Release
		if _, err := client.REST(http.MethodPost, shared.RepoPath(r)+"/releases", req, &rel); err != nil {
			return err
		}
		if len(files) > 0 {
			if err := uploadAssets(client, r, &rel, files, false); err != nil {
				return err
			}
			if !n.draft {
				if _, err := client.REST(http.MethodPatch, fmt.Sprintf("%s/releases/%d", shared.RepoPath(r), rel.ID),
					map[string]any{"draft": false}, &rel); err != nil {
					return err
				}
			}
		}
		fmt.Fprintln(ios.Out, rel.HTMLURL)
		return nil
	}
	return c
}

func newEdit(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, newTag string
		n                noteFlags
	)
	c := &cmdutil.Command{
		Use:     "edit <tag>",
		Short:   "Edit a release",
		GroupID: "general",
		Example: `Publish a release that was previously a draft
$ eg release edit v1.0 --draft=false

Update the release notes from the content of a file
$ eg release edit v1.0 --notes-file /path/to/release_notes.md`,
		Flags: cmdutil.NewFlagSet("edit"),
		Args:  cmdutil.ExactArgs(1, "accepts 1 arg(s), received 0"),
	}
	fl := c.Flags
	addRepoFlag(c, &repoFlag)
	n.register(fl)
	fl.StringVarP(&newTag, "tag", "", "", "The name of the tag")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := n.validate(f, fl); err != nil {
			return err
		}
		req := map[string]any{}
		if fl.Changed("draft") {
			req["draft"] = n.draft
		}
		if fl.Changed("prerelease") {
			req["prerelease"] = n.prerelease
		}
		if fl.Changed("notes") {
			req["body"] = n.notes
		}
		if fl.Changed("title") {
			req["name"] = n.title
		}
		if fl.Changed("target") {
			req["target_commitish"] = n.target
		}
		if fl.Changed("tag") {
			req["tag_name"] = newTag
		}
		if len(req) == 0 {
			return cmdutil.FlagErrorf("nothing to edit; at least one flag is required")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		rel, err := findRelease(client, r, args[0])
		if err != nil {
			return err
		}
		if n.verifyTag && fl.Changed("tag") {
			ok, err := tagExists(client, r, newTag)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("tag %s doesn't exist in the repo %s, aborting due to --verify-tag flag", newTag, r.FullName())
			}
		}
		var out Release
		if _, err := client.REST(http.MethodPatch, fmt.Sprintf("%s/releases/%d", shared.RepoPath(r), rel.ID), req, &out); err != nil {
			return err
		}
		fmt.Fprintln(f.IO.Out, out.HTMLURL)
		return nil
	}
	return c
}

func confirm(f *cmdutil.Factory, yes bool, question string) error {
	if yes {
		return nil
	}
	if !f.IO.CanPrompt() {
		return cmdutil.FlagErrorf("--yes required when not running interactively")
	}
	ok, err := (&cmdutil.Prompter{IO: f.IO}).Confirm(question, false)
	if err != nil {
		return err
	}
	if !ok {
		return cmdutil.ErrCancel
	}
	return nil
}

func newDelete(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag        string
		yes, cleanupTag bool
	)
	c := &cmdutil.Command{
		Use:     "delete <tag>",
		Short:   "Delete a release",
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("delete"),
		Args:    cmdutil.ExactArgs(1, "cannot delete: release tag required"),
	}
	addRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	c.Flags.BoolVarP(&cleanupTag, "cleanup-tag", "", false, "Delete the specified tag in addition to its release")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		rel, err := findRelease(client, r, args[0])
		if err != nil {
			return err
		}
		if err := confirm(f, yes, fmt.Sprintf("Delete release %s in %s?", rel.TagName, r.FullName())); err != nil {
			return err
		}
		if _, err := client.REST(http.MethodDelete, fmt.Sprintf("%s/releases/%d", shared.RepoPath(r), rel.ID), nil, nil); err != nil {
			return err
		}
		msg := "Deleted release " + rel.TagName
		if cleanupTag {
			if _, err := client.REST(http.MethodDelete, shared.RepoPath(r)+"/git/refs/tags/"+escapePath(rel.TagName), nil, nil); err != nil {
				return fmt.Errorf("%s, but failed to delete the tag: %w", msg, err)
			}
			msg += " and tag " + rel.TagName
		}
		if f.IO.IsStderrTTY() {
			cs := f.IO.ColorScheme()
			fmt.Fprintf(f.IO.ErrOut, "%s %s\n", cs.Red("✓"), msg)
			if !cleanupTag {
				fmt.Fprintf(f.IO.ErrOut, "%s Note that the %s git tag still remains in the repository\n", cs.WarningIcon(), rel.TagName)
			}
		}
		return nil
	}
	return c
}

func newDeleteAsset(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	var yes bool
	c := &cmdutil.Command{
		Use:     "delete-asset <tag> <asset-name>",
		Short:   "Delete an asset from a release",
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("delete-asset"),
		Args:    cmdutil.ExactArgs(2, "accepts 2 arg(s)"),
	}
	addRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		rel, err := findRelease(client, r, args[0])
		if err != nil {
			return err
		}
		var asset *Asset
		for i := range rel.Assets {
			if rel.Assets[i].Name == args[1] {
				asset = &rel.Assets[i]
			}
		}
		if asset == nil {
			return fmt.Errorf("asset %s not found in release %s", args[1], rel.TagName)
		}
		if err := confirm(f, yes, fmt.Sprintf("Delete asset %s in release %s in %s?", asset.Name, rel.TagName, r.FullName())); err != nil {
			return err
		}
		if _, err := client.REST(http.MethodDelete, fmt.Sprintf("%s/releases/assets/%d", shared.RepoPath(r), asset.ID), nil, nil); err != nil {
			return err
		}
		if f.IO.IsStderrTTY() {
			fmt.Fprintf(f.IO.ErrOut, "%s Deleted asset %s from release %s\n", f.IO.ColorScheme().Red("✓"), asset.Name, rel.TagName)
		}
		return nil
	}
	return c
}
