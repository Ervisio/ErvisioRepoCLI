// Package search implements `eg search`.
package search

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// New returns the search command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:   "search <command>",
		Short: "Search for repositories, issues, and pull requests",
		Long: `Search across ErvisioRepo for repositories, issues, pull requests and code.

The server searches issues, pull requests and code within one repository:
pass --repo, or run the command inside a clone so the repository is
inferred from the git remotes.

Commit search is not available.`,
		GroupID: "core",
	}
	c.AddCommand(newRepos(f), newIssues(f, false), newIssues(f, true), newCode(f), newCommits())
	return c
}

// query builds the search string: keywords followed by qualifiers.
type query struct {
	keywords []string
	quals    []string
}

func (q *query) add(key, value string) {
	if value == "" {
		return
	}
	if strings.ContainsAny(value, " \t") {
		value = strconv.Quote(value)
	}
	q.quals = append(q.quals, key+":"+value)
}

func (q *query) String() string {
	return strings.TrimSpace(strings.Join(append(append([]string(nil), q.keywords...), q.quals...), " "))
}

// hasQual reports whether the keywords already carry a qualifier.
func (q *query) hasQual(key string) bool {
	for _, k := range q.keywords {
		if strings.HasPrefix(strings.ToLower(k), key+":") {
			return true
		}
	}
	return false
}

// page is a REST search answer.
type page[T any] struct {
	TotalCount        int  `json:"total_count"`
	IncompleteResults bool `json:"incomplete_results"`
	Items             []T  `json:"items"`
}

// run pages through a search endpoint until limit items are read.
func run[T any](c *api.Client, kind, q string, limit int) ([]T, int, error) {
	v := url.Values{}
	v.Set("q", q)
	v.Set("per_page", strconv.Itoa(min(max(limit, 1), 100)))
	next := "search/" + kind + "?" + v.Encode()
	var (
		items []T
		total int
	)
	for next != "" && len(items) < limit {
		var p page[T]
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &p); err != nil {
			if api.IsNotFound(err) || api.IsStatus(err, http.StatusNotImplemented) {
				return nil, 0, cmdutil.Unsupported("Searching "+kind, "GET /api/v3/search/"+kind)
			}
			return nil, 0, err
		}
		total = p.TotalCount
		items = append(items, p.Items...)
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

// searchHost picks the host of the search: the repository's, else the
// default host.
func searchHost(f *cmdutil.Factory, repos []ghrepo.Repo) (string, error) {
	if len(repos) > 0 {
		return repos[0].Host, nil
	}
	return f.DefaultHost()
}

// parseRepos parses --repo values.
func parseRepos(f *cmdutil.Factory, values []string) ([]ghrepo.Repo, error) {
	var out []ghrepo.Repo
	for _, v := range values {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s == "" {
				continue
			}
			r, err := f.BaseRepo(s)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// needRepo resolves the repositories an issue or code search runs in:
// --repo, a repo: keyword, else the repository of the current directory.
func needRepo(f *cmdutil.Factory, q *query, repos []ghrepo.Repo, what string) ([]ghrepo.Repo, error) {
	if len(repos) > 0 || q.hasQual("repo") {
		return repos, nil
	}
	r, err := f.BaseRepo("")
	if err != nil {
		return nil, fmt.Errorf("this ErvisioRepo server searches %s within one repository: pass --repo OWNER/REPO "+
			"or a repo:OWNER/REPO qualifier, or run the command inside a clone (%w)", what, err)
	}
	return []ghrepo.Repo{r}, nil
}

// perRepo runs one search per repository (the server takes one repo:
// qualifier) and concatenates the results.
func perRepo[T any](c *api.Client, kind string, q *query, repos []ghrepo.Repo, limit int) ([]T, int, error) {
	if len(repos) == 0 {
		return run[T](c, kind, q.String(), limit)
	}
	var (
		all   []T
		total int
	)
	for _, r := range repos {
		qq := *q
		qq.quals = append([]string{"repo:" + r.FullName()}, q.quals...)
		items, n, err := run[T](c, kind, qq.String(), limit-len(all))
		if err != nil {
			return nil, 0, err
		}
		all, total = append(all, items...), total+n
		if len(all) >= limit {
			break
		}
	}
	return all, total, nil
}

func header(f *cmdutil.Factory, shown, total int, noun string) {
	if f.IO.IsStdoutTTY() {
		fmt.Fprintf(f.IO.Out, "\nShowing %d of %d %s\n\n", shown, total, noun)
	}
}

func noResults(f *cmdutil.Factory, noun string) error {
	if f.IO.IsStdoutTTY() {
		fmt.Fprintf(f.IO.ErrOut, "no %s matched your search\n", noun)
	}
	return cmdutil.ErrSilent
}

func ago(f *cmdutil.Factory, ts string) string {
	if !f.IO.IsStdoutTTY() {
		return ts
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return cmdutil.FuzzyAgo(time.Now(), t)
}

func newCommits() *cmdutil.Command {
	return &cmdutil.Command{
		Use:   "commits [<query>]",
		Short: "Search for commits",
		Long:  "Search for commits. Not supported: the ErvisioRepo server has no commit search.",
		Flags: cmdutil.NewFlagSet("commits"),
		Run: func(*cmdutil.Command, []string) error {
			return cmdutil.Unsupported("Commit search", "GET /api/v3/search/commits")
		},
	}
}

var errNoQuery = errors.New("specify search keywords or flags")
