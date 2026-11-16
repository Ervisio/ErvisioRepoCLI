package release

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

var listJSONFields = []string{"createdAt", "isDraft", "isLatest", "isPrerelease", "name", "publishedAt", "tagName"}

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, order                 string
		limit                           int
		excludeDrafts, excludePrereleas bool
		exporter                        cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "list",
		Short:   "List releases in a repository",
		Aliases: []string{"ls"},
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("list"),
		Args:    cmdutil.NoArgs,
	}
	fl := c.Flags
	addRepoFlag(c, &repoFlag)
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of items to fetch")
	fl.BoolVarP(&excludeDrafts, "exclude-drafts", "", false, "Exclude draft releases")
	fl.BoolVarP(&excludePrereleas, "exclude-pre-releases", "", false, "Exclude pre-releases")
	fl.EnumVarP(&order, "order", "O", "desc", []string{"asc", "desc"}, "Order of releases returned")
	cmdutil.AddJSONFlags(c, &exporter, listJSONFields)
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if limit < 1 {
			return cmdutil.FlagErrorf("invalid limit: %v", limit)
		}
		if err := exporter.Validate(); err != nil {
			return err
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		keep := func(x *Release) bool { return (!excludeDrafts || !x.Draft) && (!excludePrereleas || !x.Prerelease) }
		list, _, _, err := shared.Collect(client, shared.RepoPath(r)+"/releases", limit, keep)
		if err != nil {
			return err
		}
		latest := ""
		var lr Release
		if _, err := client.REST(http.MethodGet, shared.RepoPath(r)+"/releases/latest", nil, &lr); err == nil {
			latest = lr.TagName
		}
		if order == "asc" {
			sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt < list[j].CreatedAt })
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(list))
			for i, x := range list {
				m := x.export()
				m["isLatest"] = x.TagName == latest
				out[i] = exporter.Pick(m)
			}
			return exporter.Write(f.IO, out)
		}
		if len(list) == 0 {
			if f.IO.IsStdoutTTY() {
				fmt.Fprintln(f.IO.ErrOut, "no releases found")
			}
			return cmdutil.ErrSilent
		}
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(f.IO.ErrOut, "failed to start pager: %v\n", err)
		}
		defer f.StopPager()
		cs := f.IO.ColorScheme()
		t := cmdutil.NewTable(f.IO)
		t.Header("Title", "Type", "Tag name", "Published")
		for _, x := range list {
			t.Field(x.Title(), nil, true)
			kind, color := "", cs.Gray
			switch {
			case x.Draft:
				kind, color = "Draft", cs.Red
			case x.Prerelease:
				kind, color = "Pre-release", cs.Yellow
			case x.TagName == latest:
				kind, color = "Latest", cs.Green
			}
			t.Field(kind, color, false)
			t.Field(x.TagName, nil, true)
			published := deref(x.PublishedAt)
			if published == "" {
				published = x.CreatedAt
			}
			if t.IsTTY() {
				t.Field(shared.Ago(published), cs.Gray, false)
			} else {
				t.Field(published, nil, false)
			}
			t.EndRow()
		}
		return t.Render()
	}
	return c
}
