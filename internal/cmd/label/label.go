// Package label implements `eg label`.
package label

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// New returns the label command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:   "label <command>",
		Short: "Manage labels",
		Long:  "Work with ErvisioRepo labels.",
	}
	c.AddCommand(newList(f), newCreate(f), newEdit(f), newDelete(f))
	return c
}

var labelJSONFields = []string{"color", "description", "id", "isDefault", "name", "url"}

func export(l *shared.Label, u string) map[string]any {
	return map[string]any{"color": l.Color, "description": l.Desc(), "id": l.NodeID, "isDefault": l.Default,
		"name": l.Name, "url": u}
}

func labelPath(name string) string { return "/labels/" + url.PathEscape(name) }

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, search, sortBy, order string
		limit                           int
		exporter                        cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "list",
		Short: "List labels in a repository",
		Long: `Display labels in an ErvisioRepo repository.

When using the --search flag results are sorted by best match of the query.`,
		Aliases: []string{"ls"},
		Example: `# sort labels by name
$ eg label list --sort name

# find labels with "bug" in the name or description
$ eg label list --search bug`,
		Flags: cmdutil.NewFlagSet("list"),
		Args:  cmdutil.NoArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of labels to fetch")
	fl.StringVarP(&search, "search", "S", "", "Search label names and descriptions")
	fl.EnumVarP(&sortBy, "sort", "", "created", []string{"created", "name"}, "Sort fetched labels")
	fl.EnumVarP(&order, "order", "", "asc", []string{"asc", "desc"}, "Order of labels returned")
	cmdutil.AddJSONFlags(c, &exporter, labelJSONFields)
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if limit < 1 {
			return cmdutil.FlagErrorf("invalid limit: %v", limit)
		}
		if err := exporter.Validate(); err != nil {
			return err
		}
		if search != "" && (fl.Changed("sort") || fl.Changed("order")) {
			return cmdutil.FlagErrorf("cannot specify `--order` or `--sort` with `--search`")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		q := strings.ToLower(search)
		keep := func(l *shared.Label) bool {
			return q == "" || strings.Contains(strings.ToLower(l.Name), q) || strings.Contains(strings.ToLower(l.Desc()), q)
		}
		all, total, more, err := shared.Collect(client, shared.RepoPath(r)+"/labels", 1<<30, keep)
		if err != nil {
			return err
		}
		if sortBy == "name" {
			sort.SliceStable(all, func(i, j int) bool { return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name) })
		} else {
			sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
		}
		if order == "desc" {
			for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
				all[i], all[j] = all[j], all[i]
			}
		}
		if len(all) > limit {
			all = all[:limit]
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(all))
			for i, l := range all {
				out[i] = exporter.Pick(export(l, f.WebURL(r, "issues")+"?labels="+url.QueryEscape(l.Name)))
			}
			return exporter.Write(f.IO, out)
		}
		ios := f.IO
		if len(all) == 0 {
			if ios.IsStdoutTTY() {
				if search != "" {
					fmt.Fprintf(ios.ErrOut, "no labels in %s matched your search\n", r.FullName())
				} else {
					fmt.Fprintf(ios.ErrOut, "no labels found in %s\n", r.FullName())
				}
			}
			return cmdutil.ErrSilent
		}
		if ios.IsStdoutTTY() {
			of := fmt.Sprint(total)
			if more {
				of = "more than " + of
			}
			fmt.Fprintf(ios.Out, "\nShowing %d of %s labels in %s\n\n", len(all), of, r.FullName())
		}
		cs := ios.ColorScheme()
		t := cmdutil.NewTable(ios)
		t.Header("Name", "Description", "Color")
		for _, l := range all {
			color := l.Color
			t.Field(l.Name, func(s string) string { return cs.HexColor(color, s) }, false)
			t.Field(l.Desc(), nil, true)
			t.Field("#"+l.Color, func(s string) string { return cs.HexColor(color, s) }, false)
			t.EndRow()
		}
		return t.Render()
	}
	return c
}

// randomColor picks one of gh's label colors.
func randomColor() string {
	colors := []string{"B60205", "D93F0B", "FBCA04", "0E8A16", "006B75", "1D76DB", "0052CC", "5319E7",
		"E99695", "F9D0C4", "FEF2C0", "C2E0C6", "BFDADC", "C5DEF5", "BFD4F2", "D4C5F9"}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(colors))))
	if err != nil {
		return colors[0]
	}
	return colors[n.Int64()]
}

func newCreate(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, color, desc string
		force                 bool
	)
	c := &cmdutil.Command{
		Use:   "create <name>",
		Short: "Create a new label",
		Long: `Create a new label on ErvisioRepo, or update an existing one with --force.

Must specify name for the label. The description and color are optional.
If a color isn't provided, a random one will be chosen.

The label color needs to be 6 character hex value.`,
		Example: `# create new bug label
$ eg label create bug --description "Something isn't working" --color E99695`,
		Flags: cmdutil.NewFlagSet("create"),
		Args:  cmdutil.ExactArgs(1, "cannot create label: name argument required"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.StringVarP(&color, "color", "c", "", "Color of the label")
	c.Flags.StringVarP(&desc, "description", "d", "", "Description of the label")
	c.Flags.BoolVarP(&force, "force", "f", false, "Update the label color and description if label already exists")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if color == "" {
			color = randomColor()
		}
		color = strings.TrimPrefix(color, "#")
		req := map[string]any{"name": args[0], "color": color, "description": desc}
		_, err = client.REST(http.MethodPost, shared.RepoPath(r)+"/labels", req, nil)
		if err != nil && isAlreadyExists(err) {
			if !force {
				return fmt.Errorf("label with name %q already exists; use `--force` to update its color and description", args[0])
			}
			upd := map[string]any{"color": color}
			if c.Flags.Changed("description") {
				upd["description"] = desc
			}
			_, err = client.REST(http.MethodPatch, shared.RepoPath(r)+labelPath(args[0]), upd, nil)
		}
		if err != nil {
			return err
		}
		if f.IO.IsStderrTTY() {
			fmt.Fprintf(f.IO.ErrOut, "%s Label %q created in %s\n", f.IO.ColorScheme().SuccessIcon(), args[0], r.FullName())
		}
		return nil
	}
	return c
}

func isAlreadyExists(err error) bool {
	var he *api.HTTPError
	if !errors.As(err, &he) || (he.StatusCode != http.StatusUnprocessableEntity && he.StatusCode != http.StatusConflict) {
		return false
	}
	if he.StatusCode == http.StatusConflict {
		return true
	}
	for _, e := range he.Errors {
		if e.Code == "already_exists" {
			return true
		}
	}
	return strings.Contains(strings.ToLower(he.Message), "already exists")
}

func newEdit(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag, color, desc, newName string
	c := &cmdutil.Command{
		Use:   "edit <name>",
		Short: "Edit a label",
		Long: `Update a label on ErvisioRepo.

A label can be renamed using the --name flag.

The label color needs to be 6 character hex value.`,
		Example: `# update the color of the bug label
$ eg label edit bug --color FF0000

# rename and edit the description of the bug label
$ eg label edit bug --name big-bug --description "Bigger than normal bug"`,
		Flags: cmdutil.NewFlagSet("edit"),
		Args:  cmdutil.ExactArgs(1, "cannot update label: name argument required"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.StringVarP(&color, "color", "c", "", "Color of the label")
	c.Flags.StringVarP(&desc, "description", "d", "", "Description of the label")
	c.Flags.StringVarP(&newName, "name", "n", "", "New name of the label")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		fl := c.Flags
		req := map[string]any{}
		if fl.Changed("color") {
			req["color"] = strings.TrimPrefix(color, "#")
		}
		if fl.Changed("description") {
			req["description"] = desc
		}
		if fl.Changed("name") {
			req["new_name"] = newName
		}
		if len(req) == 0 {
			return cmdutil.FlagErrorf("specify at least one of `--color`, `--description`, or `--name`")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if _, err := client.REST(http.MethodPatch, shared.RepoPath(r)+labelPath(args[0]), req, nil); err != nil {
			if api.IsNotFound(err) {
				return fmt.Errorf("label %q not found in %s", args[0], r.FullName())
			}
			return err
		}
		if f.IO.IsStderrTTY() {
			fmt.Fprintf(f.IO.ErrOut, "%s Label %q updated in %s\n", f.IO.ColorScheme().SuccessIcon(), args[0], r.FullName())
		}
		return nil
	}
	return c
}

func newDelete(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	var yes bool
	c := &cmdutil.Command{
		Use:   "delete <name>",
		Short: "Delete a label from a repository",
		Flags: cmdutil.NewFlagSet("delete"),
		Args:  cmdutil.ExactArgs(1, "cannot delete label: name argument required"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&yes, "yes", "", false, "Confirm deletion without prompting")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		ios := f.IO
		if !yes && !ios.CanPrompt() {
			return cmdutil.FlagErrorf("--yes required when not running interactively")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		if !yes {
			ans, err := (&cmdutil.Prompter{IO: ios}).Input(fmt.Sprintf("Type %s to confirm deletion:", args[0]), "")
			if err != nil {
				return err
			}
			if ans != args[0] {
				return cmdutil.ErrCancel
			}
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if _, err := client.REST(http.MethodDelete, shared.RepoPath(r)+labelPath(args[0]), nil, nil); err != nil {
			if api.IsNotFound(err) {
				return fmt.Errorf("label %q not found in %s", args[0], r.FullName())
			}
			return err
		}
		if ios.IsStderrTTY() {
			fmt.Fprintf(ios.ErrOut, "%s Label %q deleted from %s\n", ios.ColorScheme().SuccessIcon(), args[0], r.FullName())
		}
		return nil
	}
	return c
}
