package pr

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, state, assignee, author, base, head, search string
		labels                                                []string
		limit                                                 int
		web, draft                                            bool
		exporter                                              cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "list",
		Short: "List pull requests in a repository",
		Long: `List pull requests in an ErvisioRepo repository. By default, this only lists open PRs.

The search query syntax supports the repo:, is:, state:, author:, assignee:,
mentions:, label:, milestone:, base:, head:, no:, sort: and
review-requested: qualifiers of the server.`,
		Aliases: []string{"ls"},
		GroupID: "general",
		Example: `# List PRs authored by you
$ eg pr list --author "@me"

# List only PRs with all of the given labels
$ eg pr list --label bug --label "priority 1"

# Filter PRs using search syntax
$ eg pr list --search "status:success review:required"

# Find a PR that introduced a given commit
$ eg pr list --state merged --base main`,
		Flags: cmdutil.NewFlagSet("list"),
		Args:  cmdutil.NoArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.StringVarP(&assignee, "assignee", "a", "", "Filter by assignee")
	fl.StringVarP(&author, "author", "A", "", "Filter by author")
	fl.StringVarP(&base, "base", "B", "", "Filter by base branch")
	fl.BoolVarP(&draft, "draft", "d", false, "Filter by draft state")
	fl.StringVarP(&head, "head", "H", "", "Filter by head branch (\"<owner>:<branch>\" syntax not supported)")
	fl.StringArrayVarP(&labels, "label", "l", "Filter by label")
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of items to fetch")
	fl.StringVarP(&search, "search", "S", "", "Search pull requests with `query`")
	fl.EnumVarP(&state, "state", "s", "open", []string{"open", "closed", "merged", "all"}, "Filter by state")
	fl.BoolVarP(&web, "web", "w", false, "List pull requests in the web browser")
	cmdutil.AddJSONFlags(c, &exporter, shared.PullJSONFields)
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if limit < 1 {
			return cmdutil.FlagErrorf("invalid value for --limit: %v", limit)
		}
		if err := exporter.Validate(); err != nil {
			return err
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		labels = shared.SplitList(labels)
		if web {
			u := f.WebURL(r, "pulls")
			if state != "all" {
				u += "?state=" + url.QueryEscape(state)
			}
			return f.OpenURL(u)
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		filtered := assignee != "" || author != "" || base != "" || head != "" || search != "" || draft || len(labels) > 0
		var (
			pulls []*shared.PullRequest
			total int
			more  bool
		)
		if search != "" {
			parts := []string{"repo:" + r.FullName(), "is:pr"}
			if state != "all" {
				parts = append(parts, "is:"+state)
			}
			for _, kv := range [][2]string{{"assignee", assignee}, {"author", author}, {"base", base}, {"head", head}} {
				if kv[1] != "" {
					parts = append(parts, kv[0]+":"+kv[1])
				}
			}
			for _, l := range labels {
				parts = append(parts, "label:"+strconv.Quote(l))
			}
			if draft {
				parts = append(parts, "draft:true")
			}
			items, count, err := shared.Search(client, strings.Join(parts, " ")+" "+search, min(limit, 100))
			if err != nil {
				return err
			}
			for i := range items {
				pulls = append(pulls, items[i].SearchToPull())
			}
			total = count
		} else {
			restState := state
			if state == "merged" {
				restState = "closed"
			}
			q := url.Values{"state": {restState}}
			if base != "" {
				q.Set("base", base)
			}
			if head != "" {
				q.Set("head", r.Owner+":"+head)
			}
			var authors, assignees []string
			if author != "" {
				if authors, err = shared.ReplaceMe(client, []string{author}); err != nil {
					return err
				}
			}
			if assignee != "" {
				if assignees, err = shared.ReplaceMe(client, []string{assignee}); err != nil {
					return err
				}
			}
			keep := func(p *shared.PullRequest) bool {
				if state == "merged" && p.StateName() != "MERGED" {
					return false
				}
				if draft && !p.Draft {
					return false
				}
				if len(authors) > 0 && !strings.EqualFold(shared.Login(p.User), authors[0]) {
					return false
				}
				if len(assignees) > 0 && !containsFold(shared.Logins(p.Assignees), assignees[0]) {
					return false
				}
				for _, l := range labels {
					if !containsFold(shared.LabelNames(p.Labels), l) {
						return false
					}
				}
				return true
			}
			pulls, total, more, err = shared.Collect(client, shared.RepoPath(r)+"/pulls?"+q.Encode(), limit, keep)
			if err != nil {
				return err
			}
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(pulls))
			for i, p := range pulls {
				m := p.ExportFields()
				if shared.WantsField(&exporter, "comments") {
					if m["comments"], err = shared.ExportComments(client, r, p.Number); err != nil {
						return err
					}
				}
				out[i] = exporter.Pick(m)
			}
			return exporter.Write(f.IO, out)
		}
		if len(pulls) == 0 {
			return shared.NoResults(f.IO, "pull requests", state, r, filtered)
		}
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(f.IO.ErrOut, "failed to start pager: %v\n", err)
		}
		defer f.StopPager()
		shared.ListHeader(f.IO, len(pulls), total, more, "pull requests", state, r, filtered)
		cs := f.IO.ColorScheme()
		t := cmdutil.NewTable(f.IO)
		t.Header("ID", "Title", "Branch", "Created at")
		for _, p := range pulls {
			color := shared.PullStateColor(cs, p.StateName(), p.Draft)
			if t.IsTTY() {
				t.Field("#"+strconv.Itoa(p.Number), color, false)
			} else {
				t.Field(strconv.Itoa(p.Number), nil, false)
			}
			t.Field(p.Title, nil, true)
			t.Field(p.Head.Ref, cs.Cyan, true)
			if t.IsTTY() {
				t.Field(shared.Ago(p.CreatedAt), cs.Gray, false)
			} else {
				st := p.StateName()
				if p.Draft && st == "OPEN" {
					st = "DRAFT"
				}
				t.Field(st, nil, false)
				t.Field(p.CreatedAt, nil, false)
			}
			t.EndRow()
		}
		return t.Render()
	}
	return c
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
