// Package run implements `eg run` and the Actions types shared with
// `eg workflow`.
package run

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

// New returns the run command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "run <command>",
		Short:   "View details about workflow runs",
		Long:    "List, view, and watch recent workflow runs from Ervisio Actions.",
		GroupID: "actions",
	}
	c.AddCommand(newList(f), newView(f), newWatch(f), newRerun(f), newCancel(f), newDownload(f), newDelete(f))
	return c
}

// Run is a REST workflow run.
type Run struct {
	ID           int64        `json:"id"`
	Name         string       `json:"name"`
	DisplayTitle string       `json:"display_title"`
	HeadBranch   *string      `json:"head_branch"`
	HeadSHA      string       `json:"head_sha"`
	Path         string       `json:"path"`
	RunNumber    int64        `json:"run_number"`
	Event        string       `json:"event"`
	Status       string       `json:"status"`
	Conclusion   *string      `json:"conclusion"`
	WorkflowID   int64        `json:"workflow_id"`
	URL          string       `json:"url"`
	HTMLURL      string       `json:"html_url"`
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
	RunStartedAt *string      `json:"run_started_at"`
	RunAttempt   int          `json:"run_attempt"`
	Actor        *shared.User `json:"actor"`
	// External marks a run imported from GitHub: its jobs, logs and
	// artifacts stay on GitHub (html_url).
	External bool `json:"external"`
}

// Branch is the head branch or "".
func (r *Run) Branch() string {
	if r.HeadBranch == nil {
		return ""
	}
	return *r.HeadBranch
}

// Conc is the conclusion or "".
func (r *Run) Conc() string { return deref(r.Conclusion) }

// Title is the display title, else the workflow name.
func (r *Run) Title() string {
	if r.DisplayTitle != "" {
		return r.DisplayTitle
	}
	return r.Name
}

// Completed reports whether the run has finished.
func (r *Run) Completed() bool { return r.Status == "completed" }

// Failed reports a completed run that did not succeed.
func (r *Run) Failed() bool {
	if !r.Completed() {
		return false
	}
	switch r.Conc() {
	case "success", "neutral", "skipped":
		return false
	}
	return true
}

func (r *Run) started() time.Time {
	if r.RunStartedAt != nil {
		if t, err := time.Parse(time.RFC3339, *r.RunStartedAt); err == nil {
			return t
		}
	}
	return parseTime(r.CreatedAt)
}

// Elapsed is the duration of the run (until now while it runs).
func (r *Run) Elapsed(now time.Time) time.Duration {
	end := parseTime(r.UpdatedAt)
	if !r.Completed() {
		end = now
	}
	d := end.Sub(r.started())
	if d < 0 {
		return 0
	}
	return d
}

// Job is a REST job of a run.
type Job struct {
	ID          int64   `json:"id"`
	RunID       int64   `json:"run_id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Conclusion  *string `json:"conclusion"`
	StartedAt   *string `json:"started_at"`
	CompletedAt *string `json:"completed_at"`
	HTMLURL     string  `json:"html_url"`
	Steps       []Step  `json:"steps"`
}

// Step is a step of a job.
type Step struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Conclusion  *string `json:"conclusion"`
	Number      int     `json:"number"`
	StartedAt   *string `json:"started_at"`
	CompletedAt *string `json:"completed_at"`
}

// Artifact is a REST artifact.
type Artifact struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	SizeInBytes        int64  `json:"size_in_bytes"`
	ArchiveDownloadURL string `json:"archive_download_url"`
	Expired            bool   `json:"expired"`
	WorkflowRun        *struct {
		ID int64 `json:"id"`
	} `json:"workflow_run"`
}

// Workflow is a REST workflow.
type Workflow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

// Base is the file name of the workflow.
func (w *Workflow) Base() string {
	if i := strings.LastIndex(w.Path, "/"); i >= 0 {
		return w.Path[i+1:]
	}
	return w.Path
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// ActionsPath is the Actions API prefix of r.
func ActionsPath(r ghrepo.Repo) string { return shared.RepoPath(r) + "/actions" }

// Symbol renders the status of a run, job or step as gh does.
func Symbol(cs *iostreams.ColorScheme, status, conclusion string) string {
	if status != "completed" {
		return cs.Yellow("*")
	}
	switch conclusion {
	case "success":
		return cs.Green("✓")
	case "skipped", "cancelled", "neutral":
		return cs.Gray("-")
	case "action_required":
		return cs.Yellow("!")
	}
	return cs.Red("X")
}

// FetchRun loads a run (attempt 0 is the latest).
func FetchRun(c *api.Client, r ghrepo.Repo, id int64, attempt int) (*Run, error) {
	p := ActionsPath(r) + "/runs/" + strconv.FormatInt(id, 10)
	if attempt > 0 {
		p += "/attempts/" + strconv.Itoa(attempt)
	}
	var run Run
	if _, err := c.REST(http.MethodGet, p, nil, &run); err != nil {
		if api.IsNotFound(err) {
			return nil, fmt.Errorf("could not find any workflow run with ID %d", id)
		}
		return nil, err
	}
	return &run, nil
}

// FetchJobs loads every job of a run.
func FetchJobs(c *api.Client, r ghrepo.Repo, id int64, attempt int) ([]Job, error) {
	p := ActionsPath(r) + "/runs/" + strconv.FormatInt(id, 10)
	if attempt > 0 {
		p += "/attempts/" + strconv.Itoa(attempt)
	}
	next := p + "/jobs?per_page=100"
	var jobs []Job
	for next != "" {
		var page struct {
			Jobs []Job `json:"jobs"`
		}
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		jobs = append(jobs, page.Jobs...)
	}
	return jobs, nil
}

// Filter selects runs of a listing.
type Filter struct {
	WorkflowID int64
	Branch     string
	Actor      string
	Event      string
	Status     string
	Created    string
	Commit     string
}

// ListRuns returns up to limit runs, newest first, and the total count.
func ListRuns(c *api.Client, r ghrepo.Repo, flt Filter, limit int) ([]Run, int, error) {
	p := ActionsPath(r) + "/runs"
	if flt.WorkflowID != 0 {
		p = ActionsPath(r) + "/workflows/" + strconv.FormatInt(flt.WorkflowID, 10) + "/runs"
	}
	q := url.Values{}
	for k, v := range map[string]string{"branch": flt.Branch, "actor": flt.Actor, "event": flt.Event,
		"status": flt.Status, "created": flt.Created, "head_sha": flt.Commit} {
		if v != "" {
			q.Set(k, v)
		}
	}
	q.Set("per_page", strconv.Itoa(min(max(limit, 1), 100)))
	next := p + "?" + q.Encode()
	var (
		runs  []Run
		total int
	)
	for next != "" && len(runs) < limit {
		var page struct {
			TotalCount int   `json:"total_count"`
			Runs       []Run `json:"workflow_runs"`
		}
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &page); err != nil {
			return nil, 0, err
		}
		total = page.TotalCount
		runs = append(runs, page.Runs...)
	}
	if len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, total, nil
}

// ListWorkflows returns every workflow of r.
func ListWorkflows(c *api.Client, r ghrepo.Repo) ([]Workflow, error) {
	next := ActionsPath(r) + "/workflows?per_page=100"
	var out []Workflow
	for next != "" {
		var page struct {
			Workflows []Workflow `json:"workflows"`
		}
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Workflows...)
	}
	return out, nil
}

// FindWorkflow resolves a workflow ID, name or file name. On a terminal
// several matches are offered in a prompt.
func FindWorkflow(f *cmdutil.Factory, c *api.Client, r ghrepo.Repo, sel string) (*Workflow, error) {
	if id, err := strconv.ParseInt(sel, 10, 64); err == nil {
		var w Workflow
		if _, err := c.REST(http.MethodGet, ActionsPath(r)+"/workflows/"+strconv.FormatInt(id, 10), nil, &w); err != nil {
			if api.IsNotFound(err) {
				return nil, fmt.Errorf("could not find any workflows named %s", sel)
			}
			return nil, err
		}
		return &w, nil
	}
	all, err := ListWorkflows(c, r)
	if err != nil {
		return nil, err
	}
	var matches []Workflow
	for _, w := range all {
		if strings.EqualFold(w.Name, sel) || w.Base() == sel || w.Path == sel {
			matches = append(matches, w)
		}
	}
	switch {
	case len(matches) == 1:
		return &matches[0], nil
	case len(matches) == 0:
		return nil, fmt.Errorf("could not find any workflows named %s", sel)
	}
	if !f.IO.CanPrompt() {
		var names []string
		for _, w := range matches {
			names = append(names, w.Base())
		}
		return nil, fmt.Errorf("could not resolve to a unique workflow; found: %s", strings.Join(names, " "))
	}
	opts := make([]string, len(matches))
	for i, w := range matches {
		opts[i] = fmt.Sprintf("%s (%s)", w.Name, w.Base())
	}
	i, err := (&cmdutil.Prompter{IO: f.IO}).Select("Which workflow do you mean?", 0, opts)
	if err != nil {
		return nil, err
	}
	return &matches[i], nil
}

// pickRun asks for a run on a terminal; keep filters the candidates.
func pickRun(f *cmdutil.Factory, c *api.Client, r ghrepo.Repo, keep func(*Run) bool, none string) (int64, error) {
	if !f.IO.CanPrompt() {
		return 0, cmdutil.FlagErrorf("run or job ID required when not running interactively")
	}
	runs, _, err := ListRuns(c, r, Filter{}, 30)
	if err != nil {
		return 0, err
	}
	var cand []Run
	for i := range runs {
		if keep == nil || keep(&runs[i]) {
			cand = append(cand, runs[i])
		}
	}
	if len(cand) == 0 {
		return 0, errors.New(none)
	}
	cs := f.IO.ColorScheme()
	now := time.Now()
	opts := make([]string, len(cand))
	for i, run := range cand {
		opts[i] = fmt.Sprintf("%s %s, %s [%s] %s", Symbol(cs, run.Status, run.Conc()), run.Title(), run.Name,
			run.Branch(), cmdutil.FuzzyAgo(now, parseTime(run.CreatedAt)))
	}
	i, err := (&cmdutil.Prompter{IO: f.IO}).Select("Select a workflow run", 0, opts)
	if err != nil {
		return 0, err
	}
	return cand[i].ID, nil
}

// parseRunID parses a run ID argument (also the URL of a run).
func parseRunID(s string) (int64, error) {
	if i := strings.Index(s, "/actions/runs/"); i >= 0 {
		s = s[i+len("/actions/runs/"):]
		s, _, _ = strings.Cut(s, "/")
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid run ID %q", s)
	}
	return id, nil
}

// externalNote explains where the data of an imported run lives.
func externalNote(run *Run, what string) error {
	return fmt.Errorf("run %d was imported from GitHub: its %s are on GitHub, see %s", run.ID, what, run.HTMLURL)
}

// FormatDuration renders a duration as gh: 1h2m3s, 4m5s, 6s.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func jobDuration(j *Job) time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	start := parseTime(*j.StartedAt)
	end := time.Now()
	if j.CompletedAt != nil {
		end = parseTime(*j.CompletedAt)
	}
	if d := end.Sub(start); d > 0 {
		return d
	}
	return 0
}

// runExport maps a run to gh's JSON fields.
func runExport(run *Run) map[string]any {
	started := run.CreatedAt
	if run.RunStartedAt != nil {
		started = *run.RunStartedAt
	}
	return map[string]any{
		"attempt": run.RunAttempt, "conclusion": run.Conc(), "createdAt": run.CreatedAt, "databaseId": run.ID,
		"displayTitle": run.Title(), "event": run.Event, "headBranch": run.Branch(), "headSha": run.HeadSHA,
		"name": run.Name, "number": run.RunNumber, "startedAt": started, "status": run.Status,
		"updatedAt": run.UpdatedAt, "url": run.HTMLURL, "workflowDatabaseId": run.WorkflowID,
		"workflowName": run.Name,
	}
}

func jobsExport(jobs []Job) []any {
	out := make([]any, 0, len(jobs))
	for _, j := range jobs {
		steps := make([]any, 0, len(j.Steps))
		for _, s := range j.Steps {
			steps = append(steps, map[string]any{"conclusion": deref(s.Conclusion), "name": s.Name, "number": s.Number,
				"status": s.Status, "startedAt": deref(s.StartedAt), "completedAt": deref(s.CompletedAt)})
		}
		out = append(out, map[string]any{"completedAt": deref(j.CompletedAt), "conclusion": deref(j.Conclusion),
			"databaseId": j.ID, "name": j.Name, "startedAt": deref(j.StartedAt), "status": j.Status,
			"steps": steps, "url": j.HTMLURL})
	}
	return out
}

var runJSONFields = []string{"attempt", "conclusion", "createdAt", "databaseId", "displayTitle", "event",
	"headBranch", "headSha", "name", "number", "startedAt", "status", "updatedAt", "url",
	"workflowDatabaseId", "workflowName"}
