package issue

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, state, assignee, author, mention, milestone, search string
		labels                                                        []string
		limit                                                         int
		web                                                           bool
		exporter                                                      cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "list",
		Short:   "List issues in a repository",
		Long:    "List issues in an ErvisioRepo repository. By default, this only lists open issues.",
		Aliases: []string{"ls"},
		GroupID: "general",
		Example: `$ eg issue list --label "bug" --label "help wanted"
$ eg issue list --author monalisa
$ eg issue list --assignee "@me"
$ eg issue list --milestone "The big 1.0"
$ eg issue list --search "error no:assignee sort:created-asc"
$ eg issue list --state all`,
		Flags: cmdutil.NewFlagSet("list"),
		Args:  cmdutil.NoArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.StringVarP(&assignee, "assignee", "a", "", "Filter by assignee")
	fl.StringVarP(&author, "author", "A", "", "Filter by author")
	fl.StringArrayVarP(&labels, "label", "l", "Filter by label")
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of issues to fetch")
	fl.StringVarP(&mention, "mention", "", "", "Filter by mention")
	fl.StringVarP(&milestone, "milestone", "m", "", "Filter by milestone number or title")
	fl.StringVarP(&search, "search", "S", "", "Search issues with `query`")
	fl.EnumVarP(&state, "state", "s", "open", []string{"open", "closed", "all"}, "Filter by state")
	fl.BoolVarP(&web, "web", "w", false, "List issues in the web browser")
	cmdutil.AddJSONFlags(c, &exporter, shared.IssueJSONFields)
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
		labels = shared.SplitList(labels)
		if web {
			q := url.Values{}
			if state != "all" {
				q.Set("state", state)
			}
			setIf(q, "q", search)
			setIf(q, "labels", strings.Join(labels, ","))
			setIf(q, "milestone", milestone)
			setIf(q, "assignee", assignee)
			setIf(q, "author", author)
			setIf(q, "mentioned", mention)
			u := f.WebURL(r, "issues")
			if len(q) > 0 {
				u += "?" + q.Encode()
			}
			return f.OpenURL(u)
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		filtered := assignee != "" || author != "" || mention != "" || milestone != "" || search != "" || len(labels) > 0
		var (
			issues []*shared.Issue
			total  int
			more   bool
		)
		if search != "" {
			q := searchQuery(r, "issue", state, search, assignee, author, mention, milestone, labels)
			items, count, err := shared.Search(client, q, min(limit, 100))
			if err != nil {
				return err
			}
			for i := range items {
				issues = append(issues, items[i].SearchToIssue())
			}
			total = count
		} else {
			q := url.Values{"state": {state}}
			setIf(q, "labels", strings.Join(labels, ","))
			for _, k := range [][2]string{{"assignee", assignee}, {"creator", author}, {"mentioned", mention}} {
				if k[1] == "" {
					continue
				}
				v, err := shared.ReplaceMe(client, []string{k[1]})
				if err != nil {
					return err
				}
				q.Set(k[0], strings.Join(v, ","))
			}
			if milestone != "" {
				n, err := milestoneParam(client, r, milestone)
				if err != nil {
					return err
				}
				q.Set("milestone", n)
			}
			issues, total, more, err = shared.Collect[shared.Issue](client, shared.RepoPath(r)+"/issues?"+q.Encode(), limit,
				func(i *shared.Issue) bool { return i.PullRequest == nil })
			if err != nil {
				return err
			}
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(issues))
			for i, is := range issues {
				m := is.ExportFields()
				if shared.WantsField(&exporter, "comments") {
					if m["comments"], err = shared.ExportComments(client, r, is.Number); err != nil {
						return err
					}
				}
				out[i] = exporter.Pick(m)
			}
			return exporter.Write(f.IO, out)
		}
		if len(issues) == 0 {
			return shared.NoResults(f.IO, "issues", state, r, filtered)
		}
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(f.IO.ErrOut, "failed to start pager: %v\n", err)
		}
		defer f.StopPager()
		shared.ListHeader(f.IO, len(issues), total, more, "issues", state, r, filtered)
		return printIssues(f, issues)
	}
	return c
}

func setIf(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}

// milestoneParam turns a milestone title into the REST number.
func milestoneParam(c *api.Client, r ghrepo.Repo, m string) (string, error) {
	switch m {
	case "*", "none":
		return m, nil
	}
	if _, err := strconv.Atoi(m); err == nil {
		return m, nil
	}
	n, err := shared.MilestoneNumber(c, r, m)
	if err != nil {
		return "", fmt.Errorf("no milestone found with title %q", m)
	}
	return strconv.Itoa(n), nil
}

// searchQuery builds a GraphQL search query for the server's qualifiers.
func searchQuery(r ghrepo.Repo, kind, state, search, assignee, author, mention, milestone string, labels []string) string {
	parts := []string{"repo:" + r.FullName(), "is:" + kind}
	if state != "all" && state != "" {
		parts = append(parts, "is:"+state)
	}
	add := func(k, v string) {
		if v == "" {
			return
		}
		if strings.ContainsAny(v, " \t") {
			v = strconv.Quote(v)
		}
		parts = append(parts, k+":"+v)
	}
	add("assignee", assignee)
	add("author", author)
	add("mentions", mention)
	add("milestone", milestone)
	for _, l := range labels {
		add("label", l)
	}
	return strings.Join(parts, " ") + " " + search
}

// printIssues renders the issue table.
func printIssues(f *cmdutil.Factory, issues []*shared.Issue) error {
	cs := f.IO.ColorScheme()
	t := cmdutil.NewTable(f.IO)
	t.Header("ID", "Title", "Labels", "Updated")
	for _, i := range issues {
		color := shared.IssueStateColor(cs, i.State, i.Reason())
		if t.IsTTY() {
			t.Field("#"+strconv.Itoa(i.Number), color, false)
		} else {
			t.Field(strconv.Itoa(i.Number), nil, false)
			t.Field(strings.ToUpper(i.State), nil, false)
		}
		t.Field(i.Title, nil, true)
		t.Field(strings.Join(shared.LabelNames(i.Labels), ", "), cs.Gray, true)
		if t.IsTTY() {
			t.Field(shared.Ago(i.UpdatedAt), cs.Gray, false)
		} else {
			t.Field(i.UpdatedAt, nil, false)
		}
		t.EndRow()
	}
	return t.Render()
}
