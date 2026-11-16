// Package shared holds what `eg issue` and `eg pr` have in common: the
// REST types, argument parsing, JSON export, bodies, comments and edits.
package shared

import (
	"strings"
	"time"
)

// User is a REST user.
type User struct {
	Login  string `json:"login"`
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

func (u *User) export() any {
	if u == nil {
		return nil
	}
	return map[string]any{"id": u.NodeID, "login": u.Login, "name": u.Name, "is_bot": u.Type == "Bot"}
}

func users(list []*User) []any {
	out := []any{}
	for _, u := range list {
		if u != nil {
			out = append(out, u.export())
		}
	}
	return out
}

// Label is a REST label.
type Label struct {
	ID          int64   `json:"id"`
	NodeID      string  `json:"node_id"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description"`
	Default     bool    `json:"default"`
}

// Desc is the description or "".
func (l *Label) Desc() string {
	if l.Description == nil {
		return ""
	}
	return *l.Description
}

// Milestone is a REST milestone.
type Milestone struct {
	Number      int     `json:"number"`
	NodeID      string  `json:"node_id"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	State       string  `json:"state"`
	DueOn       *string `json:"due_on"`
}

func (m *Milestone) export() any {
	if m == nil {
		return nil
	}
	desc := ""
	if m.Description != nil {
		desc = *m.Description
	}
	due := ""
	if m.DueOn != nil {
		due = *m.DueOn
	}
	return map[string]any{"number": m.Number, "title": m.Title, "description": desc, "dueOn": due}
}

// Issue is a REST issue (also the issue side of a pull request).
type Issue struct {
	NodeID      string     `json:"node_id"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        *string    `json:"body"`
	State       string     `json:"state"`
	StateReason *string    `json:"state_reason"`
	HTMLURL     string     `json:"html_url"`
	User        *User      `json:"user"`
	Labels      []Label    `json:"labels"`
	Assignees   []*User    `json:"assignees"`
	Milestone   *Milestone `json:"milestone"`
	Locked      bool       `json:"locked"`
	Comments    int        `json:"comments"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
	ClosedAt    *string    `json:"closed_at"`
	PullRequest *struct {
		HTMLURL string `json:"html_url"`
	} `json:"pull_request"`
}

// BodyText is the body or "".
func (i *Issue) BodyText() string {
	if i.Body == nil {
		return ""
	}
	return *i.Body
}

// Reason is the lower-case state reason or "".
func (i *Issue) Reason() string {
	if i.StateReason == nil {
		return ""
	}
	return strings.ToLower(*i.StateReason)
}

// LabelNames lists the label names.
func LabelNames(ls []Label) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Name
	}
	return out
}

// Logins lists the logins of users.
func Logins(us []*User) []string {
	var out []string
	for _, u := range us {
		if u != nil {
			out = append(out, u.Login)
		}
	}
	return out
}

func labelsExport(ls []Label) []any {
	out := []any{}
	for _, l := range ls {
		out = append(out, map[string]any{"id": l.NodeID, "name": l.Name, "color": l.Color, "description": l.Desc()})
	}
	return out
}

func strp(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// IssueJSONFields are the --json fields of issue list and issue view.
var IssueJSONFields = []string{
	"assignees", "author", "body", "closed", "closedAt", "comments", "createdAt", "id",
	"isPinned", "labels", "locked", "milestone", "number", "state", "stateReason", "title",
	"updatedAt", "url",
}

// ExportFields maps the issue to gh's JSON fields; comments is filled by
// the caller when requested.
func (i *Issue) ExportFields() map[string]any {
	reason := ""
	if i.StateReason != nil {
		reason = strings.ToUpper(*i.StateReason)
	}
	return map[string]any{
		"assignees":   users(i.Assignees),
		"author":      i.User.export(),
		"body":        i.BodyText(),
		"closed":      i.State == "closed",
		"closedAt":    strp(i.ClosedAt),
		"comments":    []any{},
		"createdAt":   i.CreatedAt,
		"id":          i.NodeID,
		"isPinned":    false,
		"labels":      labelsExport(i.Labels),
		"locked":      i.Locked,
		"milestone":   i.Milestone.export(),
		"number":      i.Number,
		"state":       strings.ToUpper(i.State),
		"stateReason": reason,
		"title":       i.Title,
		"updatedAt":   i.UpdatedAt,
		"url":         i.HTMLURL,
	}
}

// PullRef is the head or base of a pull request.
type PullRef struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	SHA   string `json:"sha"`
	User  *User  `json:"user"`
	Repo  *struct {
		NodeID   string `json:"node_id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
		Owner    User   `json:"owner"`
	} `json:"repo"`
}

// PullRequest is a REST pull request.
type PullRequest struct {
	NodeID              string         `json:"node_id"`
	Number              int            `json:"number"`
	Title               string         `json:"title"`
	Body                *string        `json:"body"`
	State               string         `json:"state"`
	HTMLURL             string         `json:"html_url"`
	User                *User          `json:"user"`
	Labels              []Label        `json:"labels"`
	Assignees           []*User        `json:"assignees"`
	RequestedReviewers  []*User        `json:"requested_reviewers"`
	Milestone           *Milestone     `json:"milestone"`
	Locked              bool           `json:"locked"`
	Draft               bool           `json:"draft"`
	Merged              bool           `json:"merged"`
	Mergeable           *bool          `json:"mergeable"`
	MergeableState      string         `json:"mergeable_state"`
	MergedAt            *string        `json:"merged_at"`
	MergedBy            *User          `json:"merged_by"`
	MergeCommitSHA      *string        `json:"merge_commit_sha"`
	ClosedAt            *string        `json:"closed_at"`
	CreatedAt           string         `json:"created_at"`
	UpdatedAt           string         `json:"updated_at"`
	Head                PullRef        `json:"head"`
	Base                PullRef        `json:"base"`
	AutoMerge           map[string]any `json:"auto_merge"`
	MaintainerCanModify bool           `json:"maintainer_can_modify"`
	Comments            int            `json:"comments"`
	ReviewComments      int            `json:"review_comments"`
	Commits             int            `json:"commits"`
	Additions           int            `json:"additions"`
	Deletions           int            `json:"deletions"`
	ChangedFiles        int            `json:"changed_files"`
}

// BodyText is the body or "".
func (p *PullRequest) BodyText() string {
	if p.Body == nil {
		return ""
	}
	return *p.Body
}

// StateName is gh's state: OPEN, CLOSED or MERGED.
func (p *PullRequest) StateName() string {
	if p.Merged || p.MergedAt != nil {
		return "MERGED"
	}
	return strings.ToUpper(p.State)
}

// IsCrossRepository reports a pull request from a fork.
func (p *PullRequest) IsCrossRepository() bool {
	return p.Head.Repo != nil && p.Base.Repo != nil && !strings.EqualFold(p.Head.Repo.FullName, p.Base.Repo.FullName)
}

// PullJSONFields are the --json fields of pr list and pr view.
var PullJSONFields = []string{
	"additions", "assignees", "author", "autoMergeRequest", "baseRefName", "body", "changedFiles",
	"closed", "closedAt", "comments", "createdAt", "deletions", "headRefName", "headRefOid",
	"headRepository", "headRepositoryOwner", "id", "isCrossRepository", "isDraft", "labels",
	"locked", "maintainerCanModify", "mergeCommit", "mergeStateStatus", "mergeable", "mergedAt",
	"mergedBy", "milestone", "number", "reviewRequests", "state", "title", "updatedAt", "url",
}

// ExportFields maps the pull request to gh's JSON fields.
func (p *PullRequest) ExportFields() map[string]any {
	var headRepo, headOwner, mergeCommit any
	if r := p.Head.Repo; r != nil {
		headRepo = map[string]any{"id": r.NodeID, "name": r.Name}
		headOwner = map[string]any{"id": r.Owner.NodeID, "login": r.Owner.Login}
	}
	if p.MergeCommitSHA != nil && p.StateName() == "MERGED" {
		mergeCommit = map[string]any{"oid": *p.MergeCommitSHA}
	}
	mergeable := "UNKNOWN"
	if p.Mergeable != nil {
		if *p.Mergeable {
			mergeable = "MERGEABLE"
		} else {
			mergeable = "CONFLICTING"
		}
	}
	reqs := []any{}
	for _, u := range p.RequestedReviewers {
		if u != nil {
			reqs = append(reqs, map[string]any{"__typename": "User", "login": u.Login})
		}
	}
	var auto any
	if len(p.AutoMerge) > 0 {
		auto = p.AutoMerge
	}
	return map[string]any{
		"additions":           p.Additions,
		"assignees":           users(p.Assignees),
		"author":              p.User.export(),
		"autoMergeRequest":    auto,
		"baseRefName":         p.Base.Ref,
		"body":                p.BodyText(),
		"changedFiles":        p.ChangedFiles,
		"closed":              p.State == "closed",
		"closedAt":            strp(p.ClosedAt),
		"comments":            []any{},
		"createdAt":           p.CreatedAt,
		"deletions":           p.Deletions,
		"headRefName":         p.Head.Ref,
		"headRefOid":          p.Head.SHA,
		"headRepository":      headRepo,
		"headRepositoryOwner": headOwner,
		"id":                  p.NodeID,
		"isCrossRepository":   p.IsCrossRepository(),
		"isDraft":             p.Draft,
		"labels":              labelsExport(p.Labels),
		"locked":              p.Locked,
		"maintainerCanModify": p.MaintainerCanModify,
		"mergeCommit":         mergeCommit,
		"mergeStateStatus":    strings.ToUpper(p.MergeableState),
		"mergeable":           mergeable,
		"mergedAt":            strp(p.MergedAt),
		"mergedBy":            p.MergedBy.export(),
		"milestone":           p.Milestone.export(),
		"number":              p.Number,
		"reviewRequests":      reqs,
		"state":               p.StateName(),
		"title":               p.Title,
		"updatedAt":           p.UpdatedAt,
		"url":                 p.HTMLURL,
	}
}

// Comment is an issue or pull request conversation comment.
type Comment struct {
	ID                int64  `json:"id"`
	NodeID            string `json:"node_id"`
	Body              string `json:"body"`
	User              *User  `json:"user"`
	HTMLURL           string `json:"html_url"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	AuthorAssociation string `json:"author_association"`
}

// Export maps a comment to gh's JSON shape.
func (c *Comment) Export() map[string]any {
	return map[string]any{
		"id": c.NodeID, "author": c.User.export(), "authorAssociation": c.AuthorAssociation,
		"body": c.Body, "createdAt": c.CreatedAt, "url": c.HTMLURL, "isMinimized": false,
	}
}

// Ago renders an RFC 3339 time relative to now, or the raw value on
// parse errors.
func Ago(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return fuzzyAgo(time.Now(), t)
}
