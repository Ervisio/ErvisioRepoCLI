// Package repo implements `eg repo`.
package repo

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// New returns the repo command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "repo <command>",
		Short:   "Manage repositories",
		Long:    "Work with ErvisioRepo repositories.",
		GroupID: "core",
		Example: `$ eg repo create
$ eg repo clone monalisa/hello-world
$ eg repo view --web`,
	}
	c.AddGroup(cmdutil.Group{ID: "general", Title: "GENERAL COMMANDS"}, cmdutil.Group{ID: "targeted", Title: "TARGETED COMMANDS"})
	c.AddCommand(newCreate(f), newList(f),
		newClone(f), newDelete(f), newEdit(f), newFork(f), newRename(f), newSync(f), newView(f))
	return c
}

// Repository is the REST representation of a repository.
type Repository struct {
	NodeID        string   `json:"node_id"`
	Name          string   `json:"name"`
	FullName      string   `json:"full_name"`
	Private       bool     `json:"private"`
	HTMLURL       string   `json:"html_url"`
	Description   *string  `json:"description"`
	Fork          bool     `json:"fork"`
	CloneURL      string   `json:"clone_url"`
	SSHURL        string   `json:"ssh_url"`
	Homepage      *string  `json:"homepage"`
	Language      *string  `json:"language"`
	Size          int64    `json:"size"`
	DefaultBranch string   `json:"default_branch"`
	Visibility    string   `json:"visibility"`
	Archived      bool     `json:"archived"`
	IsTemplate    bool     `json:"is_template"`
	HasIssues     bool     `json:"has_issues"`
	HasProjects   bool     `json:"has_projects"`
	HasWiki       bool     `json:"has_wiki"`
	HasDiscuss    bool     `json:"has_discussions"`
	Forks         int      `json:"forks_count"`
	Stars         int      `json:"stargazers_count"`
	Watchers      int      `json:"watchers_count"`
	Topics        []string `json:"topics"`
	PushedAt      string   `json:"pushed_at"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	AllowMerge    bool     `json:"allow_merge_commit"`
	AllowSquash   bool     `json:"allow_squash_merge"`
	AllowRebase   bool     `json:"allow_rebase_merge"`
	Owner         struct {
		Login  string `json:"login"`
		NodeID string `json:"node_id"`
		Type   string `json:"type"`
	} `json:"owner"`
	Permissions *struct {
		Admin, Maintain, Push, Triage, Pull bool
	} `json:"permissions"`
	Parent *Repository `json:"parent"`
}

func (r *Repository) description() string {
	if r.Description == nil {
		return ""
	}
	return *r.Description
}

// Ref returns the repository on host.
func (r *Repository) Ref(host string) ghrepo.Repo {
	owner, name, ok := strings.Cut(r.FullName, "/")
	if !ok {
		owner, name = r.Owner.Login, r.Name
	}
	return ghrepo.Repo{Host: host, Owner: owner, Name: name}
}

// repoJSONFields are the --json fields of repo list and repo view; they
// use gh's GraphQL names.
var repoJSONFields = []string{
	"createdAt", "defaultBranchRef", "description", "diskUsage", "forkCount",
	"hasDiscussionsEnabled", "hasIssuesEnabled", "hasProjectsEnabled", "hasWikiEnabled",
	"homepageUrl", "id", "isArchived", "isFork", "isInOrganization", "isPrivate", "isTemplate",
	"mergeCommitAllowed", "name", "nameWithOwner", "owner", "parent", "primaryLanguage",
	"pushedAt", "rebaseMergeAllowed", "repositoryTopics", "squashMergeAllowed", "sshUrl",
	"stargazerCount", "updatedAt", "url", "viewerCanAdminister", "viewerPermission",
	"visibility", "watchers",
}

// exportFields maps the REST object to gh's JSON fields.
func (r *Repository) exportFields() map[string]any {
	var lang, parent, topics any
	if r.Language != nil && *r.Language != "" {
		lang = map[string]any{"name": *r.Language}
	}
	if r.Parent != nil {
		parent = map[string]any{"id": r.Parent.NodeID, "name": r.Parent.Name,
			"owner": map[string]any{"id": r.Parent.Owner.NodeID, "login": r.Parent.Owner.Login}}
	}
	if len(r.Topics) > 0 {
		list := make([]map[string]any, len(r.Topics))
		for i, t := range r.Topics {
			list[i] = map[string]any{"name": t}
		}
		topics = list
	}
	home := ""
	if r.Homepage != nil {
		home = *r.Homepage
	}
	perm := ""
	if p := r.Permissions; p != nil {
		switch {
		case p.Admin:
			perm = "ADMIN"
		case p.Maintain:
			perm = "MAINTAIN"
		case p.Push:
			perm = "WRITE"
		case p.Triage:
			perm = "TRIAGE"
		case p.Pull:
			perm = "READ"
		}
	}
	return map[string]any{
		"createdAt":             r.CreatedAt,
		"defaultBranchRef":      map[string]any{"name": r.DefaultBranch},
		"description":           r.description(),
		"diskUsage":             r.Size,
		"forkCount":             r.Forks,
		"hasDiscussionsEnabled": r.HasDiscuss,
		"hasIssuesEnabled":      r.HasIssues,
		"hasProjectsEnabled":    r.HasProjects,
		"hasWikiEnabled":        r.HasWiki,
		"homepageUrl":           home,
		"id":                    r.NodeID,
		"isArchived":            r.Archived,
		"isFork":                r.Fork,
		"isInOrganization":      r.Owner.Type == "Organization",
		"isPrivate":             r.Private,
		"isTemplate":            r.IsTemplate,
		"mergeCommitAllowed":    r.AllowMerge,
		"name":                  r.Name,
		"nameWithOwner":         r.FullName,
		"owner":                 map[string]any{"id": r.Owner.NodeID, "login": r.Owner.Login},
		"parent":                parent,
		"primaryLanguage":       lang,
		"pushedAt":              r.PushedAt,
		"rebaseMergeAllowed":    r.AllowRebase,
		"repositoryTopics":      topics,
		"squashMergeAllowed":    r.AllowSquash,
		"sshUrl":                r.SSHURL,
		"stargazerCount":        r.Stars,
		"updatedAt":             r.UpdatedAt,
		"url":                   r.HTMLURL,
		"viewerCanAdminister":   perm == "ADMIN",
		"viewerPermission":      perm,
		"visibility":            strings.ToUpper(r.Visibility),
		"watchers":              map[string]any{"totalCount": r.Watchers},
	}
}

// fetchRepo loads a repository.
func fetchRepo(c *api.Client, r ghrepo.Repo) (*Repository, error) {
	var out Repository
	if _, err := c.REST(http.MethodGet, repoPath(r), nil, &out); err != nil {
		if api.IsNotFound(err) {
			return nil, fmt.Errorf("could not resolve to a Repository with the name '%s'", r.FullName())
		}
		return nil, err
	}
	return &out, nil
}

func repoPath(r ghrepo.Repo) string {
	return "repos/" + url.PathEscape(r.Owner) + "/" + url.PathEscape(r.Name)
}

// viewerLogin returns the login of the authenticated user.
func viewerLogin(c *api.Client) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if _, err := c.REST(http.MethodGet, "user", nil, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// resolveArg parses a repository argument: OWNER/REPO, HOST/OWNER/REPO,
// a URL, or a bare REPO owned by the authenticated user.
func resolveArg(f *cmdutil.Factory, arg string) (ghrepo.Repo, error) {
	cfg, err := f.Config()
	if err != nil {
		return ghrepo.Repo{}, err
	}
	if ghrepo.IsURL(arg) || strings.Contains(arg, "/") {
		def, _ := cfg.DefaultHost()
		r, err := ghrepo.FromFullName(arg, def)
		if err != nil {
			return ghrepo.Repo{}, err
		}
		if h := cmdutil.MatchHost(cfg.Hosts(), r.Host); h != "" {
			r.Host = h
		}
		return r, nil
	}
	host, err := f.DefaultHost()
	if err != nil {
		return ghrepo.Repo{}, err
	}
	client, err := f.Client(host)
	if err != nil {
		return ghrepo.Repo{}, err
	}
	login, err := viewerLogin(client)
	if err != nil {
		return ghrepo.Repo{}, err
	}
	return ghrepo.New(host, login, arg), nil
}

// cloneURL returns the git URL of r with the configured protocol.
func cloneURL(f *cmdutil.Factory, r ghrepo.Repo) string {
	cfg, err := f.Config()
	if err != nil {
		return ghrepo.CloneURL(r, "https", "https")
	}
	return ghrepo.CloneURL(r, cfg.GitProtocol(r.Host), cfg.Scheme(r.Host))
}
