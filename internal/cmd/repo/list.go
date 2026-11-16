package repo

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// maxListPages bounds the pages fetched to count and sort a listing.
const maxListPages = 100

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		limit                            int
		visibility, language             string
		topics                           []string
		fork, source, archived, noArchiv bool
		exporter                         cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "list [<owner>]",
		Short:   "List repositories owned by user or organization",
		Long:    "List repositories owned by a user or organization.\n\nNote that the list will only include repositories owned by the provided argument,\nand the --fork or --source flags will not traverse ownership boundaries.",
		Aliases: []string{"ls"},
		GroupID: "general",
		Example: `# list the current user's repositories
$ eg repo list

# list the repositories of an organization
$ eg repo list my-org --limit 100

# list private repositories as JSON
$ eg repo list --visibility private --json name,url`,
		Flags: cmdutil.NewFlagSet("list"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of repositories to list")
	fl.BoolVarP(&source, "source", "", false, "Show only non-forks")
	fl.BoolVarP(&fork, "fork", "", false, "Show only forks")
	fl.StringVarP(&language, "language", "l", "", "Filter by primary coding language")
	fl.StringArrayVarP(&topics, "topic", "", "Filter by topic")
	fl.EnumVarP(&visibility, "visibility", "", "", []string{"public", "private", "internal"}, "Filter by repository visibility")
	fl.BoolVarP(&archived, "archived", "", false, "Show only archived repositories")
	fl.BoolVarP(&noArchiv, "no-archived", "", false, "Omit archived repositories")
	cmdutil.AddJSONFlags(c, &exporter, repoJSONFields)

	c.Run = func(_ *cmdutil.Command, args []string) error {
		if limit < 1 {
			return cmdutil.FlagErrorf("invalid limit: %d", limit)
		}
		if source && fork {
			return cmdutil.FlagErrorf("specify only one of `--source` or `--fork`")
		}
		if archived && noArchiv {
			return cmdutil.FlagErrorf("specify only one of `--archived` or `--no-archived`")
		}
		if err := exporter.Validate(); err != nil {
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
		owner := ""
		if len(args) > 0 {
			owner = args[0]
		} else if owner, err = viewerLogin(client); err != nil {
			return err
		}

		match := func(r *Repository) bool {
			switch {
			case visibility != "" && !strings.EqualFold(r.Visibility, visibility):
				return false
			case fork && !r.Fork, source && r.Fork:
				return false
			case archived && !r.Archived, noArchiv && r.Archived:
				return false
			case language != "" && (r.Language == nil || !strings.EqualFold(*r.Language, language)):
				return false
			}
			for _, t := range topics {
				found := false
				for _, rt := range r.Topics {
					if strings.EqualFold(rt, t) {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
			return true
		}

		var all []*Repository
		next := "users/" + url.PathEscape(owner) + "/repos?per_page=100"
		for page := 0; next != "" && page < maxListPages; page++ {
			var batch []*Repository
			if next, err = client.REST(http.MethodGet, next, nil, &batch); err != nil {
				return err
			}
			for _, r := range batch {
				if match(r) {
					all = append(all, r)
				}
			}
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].PushedAt > all[j].PushedAt })
		total := len(all)
		shown := all
		if len(shown) > limit {
			shown = shown[:limit]
		}

		if exporter.Enabled() {
			out := make([]map[string]any, len(shown))
			for i, r := range shown {
				out[i] = exporter.Pick(r.exportFields())
			}
			return exporter.Write(f.IO, out)
		}

		ios := f.IO
		cs := ios.ColorScheme()
		filtered := visibility != "" || fork || source || archived || noArchiv || language != "" || len(topics) > 0
		if ios.IsStdoutTTY() {
			switch {
			case total == 0 && filtered:
				fmt.Fprintf(ios.ErrOut, "No results match your search\n")
				return cmdutil.ErrSilent
			case total == 0:
				fmt.Fprintf(ios.ErrOut, "There are no repositories in @%s\n", owner)
				return cmdutil.ErrSilent
			}
			suffix := ""
			if filtered {
				suffix = " that match your search"
			}
			fmt.Fprintf(ios.Out, "\nShowing %d of %d repositories in @%s%s\n\n", len(shown), total, owner, suffix)
		}
		t := cmdutil.NewTable(ios)
		t.Header("Name", "Description", "Info", "Updated")
		now := time.Now()
		for _, r := range shown {
			info := r.Visibility
			if info == "" {
				info = "public"
				if r.Private {
					info = "private"
				}
			}
			if r.Fork {
				info += ", fork"
			}
			if r.Archived {
				info += ", archived"
			}
			updated := r.PushedAt
			if updated == "" {
				updated = r.UpdatedAt
			}
			t.Field(r.FullName, cs.Bold, false)
			t.Field(r.description(), nil, true)
			infoColor := cs.Gray
			if r.Private || r.Visibility == "internal" {
				infoColor = cs.Yellow
			}
			t.Field(info, infoColor, false)
			if t.IsTTY() {
				if ts, err := time.Parse(time.RFC3339, updated); err == nil {
					updated = cmdutil.FuzzyAgo(now, ts)
				}
				t.Field(updated, cs.Gray, false)
			} else {
				t.Field(updated, nil, false)
			}
			t.EndRow()
		}
		return t.Render()
	}
	return c
}
