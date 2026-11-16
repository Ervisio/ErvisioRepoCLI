package shared

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
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

func fuzzyAgo(now, t time.Time) string { return cmdutil.FuzzyAgo(now, t) }

// RepoPath is "repos/OWNER/REPO".
func RepoPath(r ghrepo.Repo) string {
	return "repos/" + url.PathEscape(r.Owner) + "/" + url.PathEscape(r.Name)
}

// ParseNumberArg reads "123", "#123" or the URL of an issue or pull
// request; the repository is set for URLs.
func ParseNumberArg(arg string) (int, *ghrepo.Repo, error) {
	if n, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil && n > 0 {
		return n, nil, nil
	}
	u, err := url.Parse(arg)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 4 {
			switch parts[2] {
			case "issues", "pull", "pulls":
				if n, err := strconv.Atoi(parts[3]); err == nil && n > 0 {
					r := ghrepo.New(u.Host, parts[0], parts[1])
					return n, &r, nil
				}
			}
		}
	}
	return 0, nil, fmt.Errorf("invalid issue or pull request number or URL: %q", arg)
}

// Target resolves the repository and number of an issue or pull request
// argument.
func Target(f *cmdutil.Factory, repoFlag, arg string) (ghrepo.Repo, int, error) {
	n, fromURL, err := ParseNumberArg(arg)
	if err != nil {
		return ghrepo.Repo{}, 0, err
	}
	if fromURL != nil {
		r := *fromURL
		if cfg, err := f.Config(); err == nil {
			if h := cmdutil.MatchHost(cfg.Hosts(), r.Host); h != "" {
				r.Host = h
			}
		}
		return r, n, nil
	}
	r, err := f.BaseRepo(repoFlag)
	return r, n, err
}

// AddRepoFlag registers -R/--repo.
func AddRepoFlag(c *cmdutil.Command, p *string) {
	c.Flags.StringVarP(p, "repo", "R", "", "Select another repository using the `[HOST/]OWNER/REPO` format")
}

// FetchIssue loads an issue; pull requests are refused unless allowPR.
func FetchIssue(c *api.Client, r ghrepo.Repo, n int) (*Issue, error) {
	var i Issue
	if _, err := c.REST(http.MethodGet, fmt.Sprintf("%s/issues/%d", RepoPath(r), n), nil, &i); err != nil {
		if api.IsNotFound(err) {
			return nil, fmt.Errorf("GraphQL: Could not resolve to an issue or pull request with the number of %d. (repository.issue)", n)
		}
		return nil, err
	}
	return &i, nil
}

// FetchPull loads a pull request.
func FetchPull(c *api.Client, r ghrepo.Repo, n int) (*PullRequest, error) {
	var p PullRequest
	if _, err := c.REST(http.MethodGet, fmt.Sprintf("%s/pulls/%d", RepoPath(r), n), nil, &p); err != nil {
		if api.IsNotFound(err) {
			return nil, fmt.Errorf("GraphQL: Could not resolve to a PullRequest with the number of %d. (repository.pullRequest)", n)
		}
		return nil, err
	}
	return &p, nil
}

// ViewerLogin returns the login of the authenticated user.
func ViewerLogin(c *api.Client) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if _, err := c.REST(http.MethodGet, "user", nil, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// ReplaceMe replaces "@me" in logins with the viewer's login.
func ReplaceMe(c *api.Client, logins []string) ([]string, error) {
	var me string
	out := make([]string, 0, len(logins))
	for _, l := range logins {
		for _, part := range strings.Split(l, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if part == "@me" {
				if me == "" {
					var err error
					if me, err = ViewerLogin(c); err != nil {
						return nil, err
					}
				}
				part = me
			}
			out = append(out, part)
		}
	}
	return out, nil
}

// SplitList flattens repeated, comma-separated flag values.
func SplitList(values []string) []string {
	var out []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// MilestoneNumber finds a milestone by title (or number).
func MilestoneNumber(c *api.Client, r ghrepo.Repo, title string) (int, error) {
	next := RepoPath(r) + "/milestones?state=all&per_page=100"
	for next != "" {
		var ms []Milestone
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &ms); err != nil {
			return 0, err
		}
		for _, m := range ms {
			if strings.EqualFold(m.Title, title) || strconv.Itoa(m.Number) == title {
				return m.Number, nil
			}
		}
	}
	return 0, fmt.Errorf("could not add to milestone '%s': '%s' not found", title, title)
}

// Collect pages through a REST list (per_page=100), keeps the items
// accepted by keep and returns at most limit of them with the total of
// matching items (counted over at most maxPages pages; more is true when
// the count stopped there).
func Collect[T any](c *api.Client, path string, limit int, keep func(*T) bool) (items []*T, total int, more bool, err error) {
	const maxPages = 10
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	next := path + sep + "per_page=100"
	for page := 0; next != ""; page++ {
		if page == maxPages {
			return items, total, true, nil
		}
		var batch []*T
		if next, err = c.REST(http.MethodGet, next, nil, &batch); err != nil {
			return nil, 0, false, err
		}
		for _, it := range batch {
			if keep != nil && !keep(it) {
				continue
			}
			total++
			if len(items) < limit {
				items = append(items, it)
			}
		}
	}
	return items, total, false, nil
}

// ListHeader prints gh's "Showing N of M ..." line on a terminal.
func ListHeader(ios *iostreams.IOStreams, shown, total int, more bool, noun, state string, r ghrepo.Repo, filtered bool) {
	if !ios.IsStdoutTTY() {
		return
	}
	of := strconv.Itoa(total)
	if more {
		of = "more than " + of
	}
	what := noun
	if state != "" && state != "all" {
		what = state + " " + noun
	}
	suffix := ""
	if filtered {
		suffix = " that match your search"
	}
	fmt.Fprintf(ios.Out, "\nShowing %d of %s %s in %s%s\n\n", shown, of, what, r.FullName(), suffix)
}

// NoResults is the error of an empty list on a terminal.
func NoResults(ios *iostreams.IOStreams, noun, state string, r ghrepo.Repo, filtered bool) error {
	if !ios.IsStdoutTTY() {
		return nil
	}
	if filtered {
		fmt.Fprintf(ios.ErrOut, "no %s match your search in %s\n", noun, r.FullName())
	} else {
		what := noun
		if state != "" && state != "all" {
			what = state + " " + noun
		}
		fmt.Fprintf(ios.ErrOut, "no %s in %s\n", what, r.FullName())
	}
	return cmdutil.ErrSilent
}

// ---- state colors ----

// IssueStateColor colors an issue state as gh: open green, closed
// purple, not planned gray.
func IssueStateColor(cs *iostreams.ColorScheme, state, reason string) func(string) string {
	switch {
	case strings.EqualFold(state, "open"):
		return cs.Green
	case strings.EqualFold(reason, "not_planned"):
		return cs.Gray
	}
	return cs.Magenta
}

// PullStateColor colors a pull request state: open green, draft gray,
// merged purple, closed red.
func PullStateColor(cs *iostreams.ColorScheme, state string, draft bool) func(string) string {
	switch {
	case strings.EqualFold(state, "open") && draft:
		return cs.Gray
	case strings.EqualFold(state, "open"):
		return cs.Green
	case strings.EqualFold(state, "merged"):
		return cs.Magenta
	}
	return cs.Red
}

// PullStateLabel is gh's state label of pr view: Open, Draft, Merged,
// Closed.
func PullStateLabel(p *PullRequest) string {
	switch {
	case p.StateName() == "MERGED":
		return "Merged"
	case p.State == "closed":
		return "Closed"
	case p.Draft:
		return "Draft"
	}
	return "Open"
}

// ---- GraphQL search ----

// SearchItem is an issue or pull request found by GraphQL search.
type SearchItem struct {
	Typename    string `json:"__typename"`
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	StateReason string `json:"stateReason"`
	URL         string `json:"url"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	IsDraft     bool   `json:"isDraft"`
	HeadRefName string `json:"headRefName"`
	Author      *struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels struct {
		Nodes []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
}

const searchQuery = `query EgSearch($q: String!, $first: Int!) {
  search(query: $q, type: ISSUE, first: $first) {
    issueCount
    nodes {
      __typename
      ... on Issue { id number title state stateReason url createdAt updatedAt author { login } labels(first: 20) { nodes { name color } } }
      ... on PullRequest { id number title state isDraft headRefName url createdAt updatedAt author { login } labels(first: 20) { nodes { name color } } }
    }
  }
}`

// Search runs a GraphQL issue search (the query must carry repo:).
func Search(c *api.Client, q string, first int) ([]SearchItem, int, error) {
	var out struct {
		Search struct {
			IssueCount int          `json:"issueCount"`
			Nodes      []SearchItem `json:"nodes"`
		} `json:"search"`
	}
	if err := c.GraphQL(searchQuery, map[string]any{"q": q, "first": first}, &out); err != nil {
		if api.IsNotFound(err) || api.IsStatus(err, http.StatusNotImplemented) {
			return nil, 0, cmdutil.Unsupported("Searching issues", "POST /api/graphql search")
		}
		return nil, 0, err
	}
	return out.Search.Nodes, out.Search.IssueCount, nil
}

// SearchToIssue converts a search hit to the REST shape.
func (s *SearchItem) SearchToIssue() *Issue {
	i := &Issue{NodeID: s.ID, Number: s.Number, Title: s.Title, State: strings.ToLower(s.State),
		HTMLURL: s.URL, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	if s.StateReason != "" {
		r := strings.ToLower(s.StateReason)
		i.StateReason = &r
	}
	if s.Author != nil {
		i.User = &User{Login: s.Author.Login}
	}
	for _, l := range s.Labels.Nodes {
		i.Labels = append(i.Labels, Label{Name: l.Name, Color: l.Color})
	}
	return i
}

// SearchToPull converts a search hit to the REST shape.
func (s *SearchItem) SearchToPull() *PullRequest {
	p := &PullRequest{NodeID: s.ID, Number: s.Number, Title: s.Title, HTMLURL: s.URL,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, Draft: s.IsDraft}
	p.Head.Ref = s.HeadRefName
	switch strings.ToUpper(s.State) {
	case "MERGED":
		p.State, p.Merged = "closed", true
	default:
		p.State = strings.ToLower(s.State)
	}
	if s.Author != nil {
		p.User = &User{Login: s.Author.Login}
	}
	for _, l := range s.Labels.Nodes {
		p.Labels = append(p.Labels, Label{Name: l.Name, Color: l.Color})
	}
	return p
}

// GraphQLMutation runs a mutation and reports a server without it as
// unsupported.
func GraphQLMutation(c *api.Client, name, query string, vars map[string]any, out any) error {
	err := c.GraphQL(query, vars, out)
	if err == nil {
		return nil
	}
	if api.IsNotFound(err) || api.IsStatus(err, http.StatusNotImplemented) {
		return cmdutil.Unsupported("The "+name+" mutation", "POST /api/graphql "+name)
	}
	var ge *api.GraphQLError
	if errors.As(err, &ge) {
		for _, e := range ge.Errors {
			if strings.Contains(e.Message, "doesn't exist on type 'Mutation'") || strings.Contains(e.Message, "unknown field") {
				return cmdutil.Unsupported("The "+name+" mutation", "POST /api/graphql "+name)
			}
		}
	}
	return err
}
