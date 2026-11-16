package search

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

type issueItem struct {
	shared.Issue
	RepositoryURL     string `json:"repository_url"`
	AuthorAssociation string `json:"author_association"`
	Draft             bool   `json:"draft"`
	PR                *struct {
		MergedAt *string `json:"merged_at"`
	} `json:"pull_request"`
}

// repoName extracts OWNER/REPO from repository_url.
func (i *issueItem) repoName() string {
	u := i.RepositoryURL
	if k := strings.Index(u, "/repos/"); k >= 0 {
		return u[k+len("/repos/"):]
	}
	return ""
}

func (i *issueItem) export() map[string]any {
	full := i.repoName()
	_, name, _ := strings.Cut(full, "/")
	labels := make([]any, 0, len(i.Labels))
	for _, l := range i.Labels {
		labels = append(labels, map[string]any{"id": l.NodeID, "name": l.Name, "color": l.Color, "description": l.Desc()})
	}
	assignees := make([]any, 0, len(i.Assignees))
	for _, a := range i.Assignees {
		if a != nil {
			assignees = append(assignees, map[string]any{"id": a.NodeID, "login": a.Login, "is_bot": a.Type == "Bot"})
		}
	}
	var author any
	if i.User != nil {
		author = map[string]any{"id": i.User.NodeID, "login": i.User.Login, "is_bot": i.User.Type == "Bot", "type": i.User.Type}
	}
	closedAt := ""
	if i.ClosedAt != nil {
		closedAt = *i.ClosedAt
	}
	return map[string]any{
		"assignees": assignees, "author": author, "authorAssociation": i.AuthorAssociation, "body": i.BodyText(),
		"closedAt": closedAt, "commentsCount": i.Comments, "createdAt": i.CreatedAt, "id": i.NodeID,
		"isDraft": i.Draft, "isLocked": i.Locked, "isPullRequest": i.PR != nil, "labels": labels, "number": i.Number,
		"repository": map[string]any{"name": name, "nameWithOwner": full}, "state": i.State, "title": i.Title,
		"updatedAt": i.UpdatedAt, "url": i.HTMLURL,
	}
}

var issueJSONFields = []string{"assignees", "author", "authorAssociation", "body", "closedAt", "commentsCount",
	"createdAt", "id", "isLocked", "isPullRequest", "labels", "number", "repository", "state", "title",
	"updatedAt", "url"}

func newIssues(f *cmdutil.Factory, prs bool) *cmdutil.Command {
	var (
		repoFlags, labels, owners, match                                []string
		author, assignee, mentions, involves, commenter, milestone      string
		state, created, updated, closed, comments, sortBy, order, app   string
		base, head, reviewRequested, reviewedBy, review, checks, merged string
		includePRs, noLabel, noAssignee, noMilestone, draft, isMerged   bool
		limit                                                           int
		exporter                                                        cmdutil.Exporter
	)
	noun, use := "issues", "issues [<query>]"
	if prs {
		noun, use = "pull requests", "prs [<query>]"
	}
	c := &cmdutil.Command{
		Use:   use,
		Short: "Search for " + noun,
		Long: `Search for ` + noun + ` on ErvisioRepo.

The command supports constructing queries using the GitHub search syntax,
using the parameter and qualifier flags, or a combination of the two.

The server searches one repository at a time: the repository comes from
--repo (several are searched one after the other), a repo: qualifier, or
the git remotes of the current directory.`,
		Example: `# search ` + noun + ` matching set of keywords "readme" and "typo"
$ eg search ` + strings.Fields(use)[0] + ` readme typo

# search open ` + noun + ` assigned to yourself
$ eg search ` + strings.Fields(use)[0] + ` --assignee=@me --state=open --repo acme/widgets`,
		Flags: cmdutil.NewFlagSet(strings.Fields(use)[0]),
	}
	if prs {
		c.Aliases = []string{"pulls", "pull-requests"}
	}
	fl := c.Flags
	fl.StringArrayVarP(&repoFlags, "repo", "R", "Filter on `repository`")
	fl.StringArrayVarP(&labels, "label", "", "Filter on label")
	fl.StringArrayVarP(&owners, "owner", "", "Filter on repository `owner`")
	fl.StringArrayVarP(&match, "match", "", "Restrict search to specific field of issue: {title|body|comments}")
	fl.StringVarP(&author, "author", "", "", "Filter by `user`")
	fl.StringVarP(&assignee, "assignee", "", "", "Filter by assignee")
	fl.StringVarP(&mentions, "mentions", "", "", "Filter based on `user` mentions")
	fl.StringVarP(&involves, "involves", "", "", "Filter based on involvement of `user`")
	fl.StringVarP(&commenter, "commenter", "", "", "Filter based on comments by `user`")
	fl.StringVarP(&milestone, "milestone", "", "", "Filter by milestone `title`")
	fl.EnumVarP(&state, "state", "", "", []string{"open", "closed"}, "Filter based on state")
	fl.StringVarP(&created, "created", "", "", "Filter based on created at `date`")
	fl.StringVarP(&updated, "updated", "", "", "Filter on last updated at `date`")
	fl.StringVarP(&closed, "closed", "", "", "Filter on closed at `date`")
	fl.StringVarP(&comments, "comments", "", "", "Filter on `number` of comments")
	fl.StringVarP(&app, "app", "", "", "Filter by GitHub App author")
	fl.BoolVarP(&noLabel, "no-label", "", false, "Filter on missing label")
	fl.BoolVarP(&noAssignee, "no-assignee", "", false, "Filter on missing assignee")
	fl.BoolVarP(&noMilestone, "no-milestone", "", false, "Filter on missing milestone")
	fl.EnumVarP(&sortBy, "sort", "", "best-match", []string{"comments", "created", "interactions", "reactions", "updated", "best-match"}, "Sort fetched results")
	fl.EnumVarP(&order, "order", "", "desc", []string{"asc", "desc"}, "Order of results returned, ignored unless '--sort' flag is specified")
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of results to fetch")
	if prs {
		fl.StringVarP(&base, "base", "B", "", "Filter on base branch `name`")
		fl.StringVarP(&head, "head", "H", "", "Filter on head branch `name`")
		fl.BoolVarP(&draft, "draft", "", false, "Filter based on draft state")
		fl.BoolVarP(&isMerged, "merged", "", false, "Filter based on merged state")
		fl.StringVarP(&merged, "merged-at", "", "", "Filter on merged at `date`")
		fl.StringVarP(&reviewRequested, "review-requested", "", "", "Filter on `user` or team requested to review")
		fl.StringVarP(&reviewedBy, "reviewed-by", "", "", "Filter on `user` who reviewed")
		fl.EnumVarP(&review, "review", "", "", []string{"none", "required", "approved", "changes_requested"}, "Filter based on review status")
		fl.EnumVarP(&checks, "checks", "", "", []string{"pending", "success", "failure"}, "Filter based on status of the checks")
	} else {
		fl.BoolVarP(&includePRs, "include-prs", "", false, "Include pull requests in results")
	}
	fields := issueJSONFields
	if prs {
		fields = append(append([]string(nil), issueJSONFields...), "isDraft")
	}
	cmdutil.AddJSONFlags(c, &exporter, fields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		if limit < 1 || limit > 1000 {
			return cmdutil.FlagErrorf("`--limit` must be between 1 and 1000")
		}
		q := &query{keywords: args}
		switch {
		case prs:
			q.add("is", "pr")
		case !includePRs:
			q.add("is", "issue")
		}
		for _, o := range owners {
			q.add("user", o)
		}
		for _, l := range labels {
			q.add("label", l)
		}
		if len(match) > 0 {
			q.add("in", strings.Join(match, ","))
		}
		q.add("author", author)
		q.add("assignee", assignee)
		q.add("mentions", mentions)
		q.add("involves", involves)
		q.add("commenter", commenter)
		q.add("milestone", milestone)
		q.add("state", state)
		q.add("created", created)
		q.add("updated", updated)
		q.add("closed", closed)
		q.add("comments", comments)
		if app != "" {
			q.add("author", "app/"+app)
		}
		if noLabel {
			q.add("no", "label")
		}
		if noAssignee {
			q.add("no", "assignee")
		}
		if noMilestone {
			q.add("no", "milestone")
		}
		if prs {
			q.add("base", base)
			q.add("head", head)
			if draft {
				q.add("draft", "true")
			}
			if isMerged {
				q.add("is", "merged")
			}
			q.add("merged", merged)
			q.add("review-requested", reviewRequested)
			q.add("reviewed-by", reviewedBy)
			q.add("review", review)
			q.add("status", checks)
		}
		if c.Flags.Changed("sort") && sortBy != "best-match" {
			q.add("sort", sortBy+"-"+order)
		}
		repos, err := parseRepos(f, repoFlags)
		if err != nil {
			return err
		}
		if repos, err = needRepo(f, q, repos, noun); err != nil {
			return err
		}
		host, err := searchHost(f, repos)
		if err != nil {
			return err
		}
		client, err := f.Client(host)
		if err != nil {
			return err
		}
		items, total, err := perRepo[issueItem](client, "issues", q, repos, limit)
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
			return noResults(f, noun)
		}
		header(f, len(items), total, noun)
		cs := f.IO.ColorScheme()
		t := cmdutil.NewTable(f.IO)
		if t.IsTTY() {
			t.Header("Repo", "ID", "Title", "Labels", "Updated")
		}
		for i := range items {
			it := &items[i]
			t.Field(it.repoName(), nil, true)
			id := "#" + strconv.Itoa(it.Number)
			state := it.State
			if it.PR != nil {
				switch {
				case it.PR.MergedAt != nil:
					state = "merged"
				case it.Draft:
					state = "draft"
				}
			}
			if t.IsTTY() {
				color := cs.Green
				switch state {
				case "closed":
					color = cs.Red
				case "merged":
					color = cs.Magenta
				case "draft":
					color = cs.Gray
				}
				t.Field(id, color, false)
			} else {
				t.Field(strconv.Itoa(it.Number), nil, false)
				t.Field(state, nil, false)
			}
			t.Field(it.Title, nil, true)
			t.Field(strings.Join(shared.LabelNames(it.Labels), ", "), nil, true)
			t.Field(ago(f, it.UpdatedAt), cs.Gray, false)
			t.EndRow()
		}
		if err := t.Render(); err != nil {
			return fmt.Errorf("render results: %w", err)
		}
		return nil
	}
	return c
}
