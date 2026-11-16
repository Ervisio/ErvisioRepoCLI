package search

import (
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

type repoItem struct {
	NodeID        string  `json:"node_id"`
	Name          string  `json:"name"`
	FullName      string  `json:"full_name"`
	Private       bool    `json:"private"`
	Fork          bool    `json:"fork"`
	Archived      bool    `json:"archived"`
	Disabled      bool    `json:"disabled"`
	HTMLURL       string  `json:"html_url"`
	Description   *string `json:"description"`
	Homepage      *string `json:"homepage"`
	Language      *string `json:"language"`
	Visibility    string  `json:"visibility"`
	DefaultBranch string  `json:"default_branch"`
	Size          int64   `json:"size"`
	Forks         int     `json:"forks_count"`
	Stars         int     `json:"stargazers_count"`
	Watchers      int     `json:"watchers_count"`
	OpenIssues    int     `json:"open_issues_count"`
	HasIssues     bool    `json:"has_issues"`
	HasWiki       bool    `json:"has_wiki"`
	HasProjects   bool    `json:"has_projects"`
	HasPages      bool    `json:"has_pages"`
	HasDownloads  bool    `json:"has_downloads"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	PushedAt      string  `json:"pushed_at"`
	License       *struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"license"`
	Owner struct {
		Login  string `json:"login"`
		NodeID string `json:"node_id"`
		Type   string `json:"type"`
		URL    string `json:"html_url"`
	} `json:"owner"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r *repoItem) export() map[string]any {
	var lic any
	if r.License != nil {
		lic = map[string]any{"key": r.License.Key, "name": r.License.Name, "url": r.License.URL}
	}
	return map[string]any{
		"createdAt": r.CreatedAt, "defaultBranch": r.DefaultBranch, "description": str(r.Description),
		"forksCount": r.Forks, "fullName": r.FullName, "hasDownloads": r.HasDownloads, "hasIssues": r.HasIssues,
		"hasPages": r.HasPages, "hasProjects": r.HasProjects, "hasWiki": r.HasWiki, "homepage": str(r.Homepage),
		"id": r.NodeID, "isArchived": r.Archived, "isDisabled": r.Disabled, "isFork": r.Fork, "isPrivate": r.Private,
		"language": str(r.Language), "license": lic, "name": r.Name, "openIssuesCount": r.OpenIssues,
		"owner":    map[string]any{"id": r.Owner.NodeID, "login": r.Owner.Login, "type": r.Owner.Type, "url": r.Owner.URL},
		"pushedAt": r.PushedAt, "size": r.Size, "stargazersCount": r.Stars, "updatedAt": r.UpdatedAt,
		"url": r.HTMLURL, "visibility": r.Visibility, "watchersCount": r.Watchers,
	}
}

var repoJSONFields = []string{"createdAt", "defaultBranch", "description", "forksCount", "fullName",
	"hasDownloads", "hasIssues", "hasPages", "hasProjects", "hasWiki", "homepage", "id", "isArchived",
	"isDisabled", "isFork", "isPrivate", "language", "license", "name", "openIssuesCount", "owner",
	"pushedAt", "size", "stargazersCount", "updatedAt", "url", "visibility", "watchersCount"}

func newRepos(f *cmdutil.Factory) *cmdutil.Command {
	var (
		owners, topics, licenses, match                       []string
		language, stars, forks, size, created, updated, vis   string
		sortBy, order, includeForks, archived, goodFirst, hlp string
		limit                                                 int
		exporter                                              cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "repos [<query>]",
		Short: "Search for repositories",
		Long: `Search for repositories on ErvisioRepo.

The command supports constructing queries using the GitHub search syntax,
using the parameter and qualifier flags, or a combination of the two.

The server matches names and descriptions and understands user:, org:,
repo: and is:public/is:private; language:, stars: and topic: give no
results.`,
		Example: `# search repositories matching set of keywords "cli" and "shell"
$ eg search repos cli shell

# search repositories matching phrase "vim plugin"
$ eg search repos "vim plugin"

# search repositories in an organization
$ eg search repos --owner=acme`,
		Flags: cmdutil.NewFlagSet("repos"),
	}
	fl := c.Flags
	fl.StringArrayVarP(&owners, "owner", "", "Filter on `owner`")
	fl.StringArrayVarP(&topics, "topic", "", "Filter on `topic`")
	fl.StringArrayVarP(&licenses, "license", "", "Filter based on license `type`")
	fl.StringArrayVarP(&match, "match", "", "Restrict search to specific field of repository: {name|description|readme}")
	fl.StringVarP(&language, "language", "", "", "Filter based on the coding language")
	fl.StringVarP(&stars, "stars", "", "", "Filter on `number` of stars")
	fl.StringVarP(&forks, "forks", "", "", "Filter on `number` of forks")
	fl.StringVarP(&size, "size", "", "", "Filter on a size range, in kilobytes")
	fl.StringVarP(&created, "created", "", "", "Filter based on created at `date`")
	fl.StringVarP(&updated, "updated", "", "", "Filter on last updated at `date`")
	fl.EnumVarP(&vis, "visibility", "", "", []string{"public", "private", "internal"}, "Filter based on visibility")
	fl.EnumVarP(&includeForks, "include-forks", "", "", []string{"false", "true", "only"}, "Include forks in fetched repositories")
	fl.EnumVarP(&archived, "archived", "", "", []string{"true", "false"}, "Filter based on the repository archived state")
	fl.StringVarP(&goodFirst, "good-first-issues", "", "", "Filter on `number` of issues with the 'good first issue' label")
	fl.StringVarP(&hlp, "help-wanted-issues", "", "", "Filter on `number` of issues with the 'help wanted' label")
	fl.EnumVarP(&sortBy, "sort", "", "best-match", []string{"forks", "help-wanted-issues", "stars", "updated", "best-match"}, "Sort fetched repositories")
	fl.EnumVarP(&order, "order", "", "desc", []string{"asc", "desc"}, "Order of repositories returned, ignored unless '--sort' flag is specified")
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of repositories to fetch")
	cmdutil.AddJSONFlags(c, &exporter, repoJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		if limit < 1 || limit > 1000 {
			return cmdutil.FlagErrorf("`--limit` must be between 1 and 1000")
		}
		q := &query{keywords: args}
		for _, o := range owners {
			q.add("user", o)
		}
		for _, t := range topics {
			q.add("topic", t)
		}
		for _, l := range licenses {
			q.add("license", l)
		}
		if len(match) > 0 {
			q.add("in", strings.Join(match, ","))
		}
		q.add("language", language)
		q.add("stars", stars)
		q.add("forks", forks)
		q.add("size", size)
		q.add("created", created)
		q.add("updated", updated)
		if vis != "" {
			q.add("is", vis)
		}
		q.add("fork", includeForks)
		q.add("archived", archived)
		q.add("good-first-issues", goodFirst)
		q.add("help-wanted-issues", hlp)
		if c.Flags.Changed("sort") && sortBy != "best-match" {
			q.add("sort", sortBy+"-"+order)
		}
		if q.String() == "" {
			return cmdutil.FlagErrorf("%s", errNoQuery)
		}
		host, err := searchHost(f, nil)
		if err != nil {
			return err
		}
		client, err := f.Client(host)
		if err != nil {
			return err
		}
		items, total, err := run[repoItem](client, "repositories", q.String(), limit)
		if err != nil {
			return err
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(items))
			for i := range items {
				out[i] = exporter.Pick(items[i].export())
			}
			return exporter.Write(f.IO, out)
		}
		if len(items) == 0 {
			return noResults(f, "repositories")
		}
		header(f, len(items), total, "repositories")
		cs := f.IO.ColorScheme()
		t := cmdutil.NewTable(f.IO)
		t.Header("Name", "Description", "Visibility", "Updated")
		for i := range items {
			r := &items[i]
			tags := []string{r.Visibility}
			if r.Fork {
				tags = append(tags, "fork")
			}
			if r.Archived {
				tags = append(tags, "archived")
			}
			info := strings.Join(tags, ", ")
			color := cs.Gray
			if r.Private {
				color = cs.Yellow
			}
			t.Field(r.FullName, cs.Bold, true)
			t.Field(str(r.Description), nil, true)
			t.Field(info, color, false)
			t.Field(ago(f, r.UpdatedAt), cs.Gray, false)
			t.EndRow()
		}
		return t.Render()
	}
	return c
}
