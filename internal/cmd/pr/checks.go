package pr

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

var checkJSONFields = []string{"bucket", "completedAt", "description", "event", "link", "name", "startedAt", "state", "workflow"}

// check is a commit status or a check run, normalized as gh does.
type check struct {
	Name        string
	State       string // SUCCESS, FAILURE, PENDING, ...
	Bucket      string // pass, fail, pending, skipping, cancel
	Description string
	Link        string
	StartedAt   time.Time
	CompletedAt time.Time
}

func (c check) export() map[string]any {
	ts := func(t time.Time) any {
		if t.IsZero() {
			return "0001-01-01T00:00:00Z"
		}
		return t.UTC().Format(time.RFC3339)
	}
	return map[string]any{"bucket": c.Bucket, "completedAt": ts(c.CompletedAt), "description": c.Description,
		"event": "", "link": c.Link, "name": c.Name, "startedAt": ts(c.StartedAt), "state": c.State, "workflow": ""}
}

func newChecks(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag                       string
		web, watch, failFast, required bool
		interval                       int
		exporter                       cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "checks " + argUse,
		Short: "Show CI status for a single pull request",
		Long: `Show CI status for a single pull request: the commit statuses and the
check runs of its head commit.

Without an argument, the pull request that belongs to the current branch
is selected.

When the --json flag is used, it includes a 'bucket' field, which
categorizes the 'state' field into 'pass', 'fail', 'pending', 'skipping',
or 'cancel'.

Additional exit codes:
  8: Checks pending`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("checks"),
		Args:    selectorArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.BoolVarP(&failFast, "fail-fast", "", false, "Exit watch mode on first check failure")
	fl.IntVarP(&interval, "interval", "i", 10, "Refresh interval in seconds when using `--watch` flag")
	fl.BoolVarP(&required, "required", "", false, "Only show checks that are required (not supported)")
	fl.BoolVarP(&watch, "watch", "", false, "Watch checks until they finish")
	fl.BoolVarP(&web, "web", "w", false, "Open the web browser to show details about checks")
	cmdutil.AddJSONFlags(c, &exporter, checkJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		if required {
			return cmdutil.Unsupported("Filtering required checks", "the isRequired field of status check contexts")
		}
		if failFast && !watch {
			return cmdutil.FlagErrorf("cannot use `--fail-fast` flag without `--watch` flag")
		}
		if watch && exporter.Enabled() {
			return cmdutil.FlagErrorf("cannot use `--watch` with `--json` flag")
		}
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		if web {
			return f.OpenURL(p.HTMLURL + "?tab=commits")
		}
		ios := f.IO
		for {
			checks, err := loadChecks(fd.client, fd.repo, p.Head.SHA)
			if err != nil {
				return err
			}
			if exporter.Enabled() {
				out := make([]map[string]any, len(checks))
				for i, ch := range checks {
					out[i] = exporter.Pick(ch.export())
				}
				return exporter.Write(ios, out)
			}
			if len(checks) == 0 {
				return fmt.Errorf("no checks reported on the '%s' branch", p.Head.Ref)
			}
			counts := map[string]int{}
			for _, ch := range checks {
				counts[ch.Bucket]++
			}
			if watch && ios.IsStdoutTTY() {
				fmt.Fprint(ios.Out, "\x1b[H\x1b[2J")
				fmt.Fprintf(ios.Out, "Refreshing checks status every %d seconds. Press Ctrl+C to quit.\n\n", interval)
			}
			if err := printChecks(ios, checks, counts); err != nil {
				return err
			}
			done := counts["pending"] == 0
			if !watch || done || failFast && counts["fail"] > 0 {
				switch {
				case counts["fail"] > 0 || counts["cancel"] > 0:
					return cmdutil.ErrSilent
				case counts["pending"] > 0:
					return cmdutil.ErrPending
				}
				return nil
			}
			time.Sleep(time.Duration(max(interval, 1)) * time.Second)
		}
	}
	return c
}

func loadChecks(c *api.Client, r ghrepo.Repo, sha string) ([]check, error) {
	var out []check
	var status struct {
		Statuses []struct {
			Context     string `json:"context"`
			State       string `json:"state"`
			Description string `json:"description"`
			TargetURL   string `json:"target_url"`
			CreatedAt   string `json:"created_at"`
			UpdatedAt   string `json:"updated_at"`
		} `json:"statuses"`
	}
	base := shared.RepoPath(r) + "/commits/" + sha
	if _, err := c.REST(http.MethodGet, base+"/status", nil, &status); err != nil {
		return nil, err
	}
	for _, s := range status.Statuses {
		ch := check{Name: s.Context, State: strings.ToUpper(s.State), Description: s.Description, Link: s.TargetURL}
		ch.StartedAt, _ = time.Parse(time.RFC3339, s.CreatedAt)
		switch s.State {
		case "success":
			ch.Bucket = "pass"
		case "failure", "error":
			ch.Bucket = "fail"
		default:
			ch.Bucket = "pending"
		}
		if ch.Bucket != "pending" {
			ch.CompletedAt, _ = time.Parse(time.RFC3339, s.UpdatedAt)
		}
		out = append(out, ch)
	}
	next := base + "/check-runs?per_page=100"
	for next != "" {
		var runs struct {
			CheckRuns []struct {
				Name        string  `json:"name"`
				Status      string  `json:"status"`
				Conclusion  *string `json:"conclusion"`
				StartedAt   *string `json:"started_at"`
				CompletedAt *string `json:"completed_at"`
				DetailsURL  string  `json:"details_url"`
				HTMLURL     string  `json:"html_url"`
				Output      struct {
					Title string `json:"title"`
				} `json:"output"`
			} `json:"check_runs"`
		}
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &runs); err != nil {
			return nil, err
		}
		for _, cr := range runs.CheckRuns {
			ch := check{Name: cr.Name, Description: cr.Output.Title, Link: cr.DetailsURL}
			if ch.Link == "" {
				ch.Link = cr.HTMLURL
			}
			if cr.StartedAt != nil {
				ch.StartedAt, _ = time.Parse(time.RFC3339, *cr.StartedAt)
			}
			if cr.CompletedAt != nil {
				ch.CompletedAt, _ = time.Parse(time.RFC3339, *cr.CompletedAt)
			}
			conclusion := ""
			if cr.Conclusion != nil {
				conclusion = *cr.Conclusion
			}
			if cr.Status != "completed" {
				ch.State, ch.Bucket = strings.ToUpper(cr.Status), "pending"
			} else {
				ch.State = strings.ToUpper(conclusion)
				switch conclusion {
				case "success", "neutral":
					ch.Bucket = "pass"
				case "skipped":
					ch.Bucket = "skipping"
				case "cancelled":
					ch.Bucket = "cancel"
				default:
					ch.Bucket = "fail"
				}
			}
			out = append(out, ch)
		}
	}
	rank := map[string]int{"fail": 0, "cancel": 1, "pending": 2, "pass": 3, "skipping": 4}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Bucket] != rank[out[j].Bucket] {
			return rank[out[i].Bucket] < rank[out[j].Bucket]
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func printChecks(ios *iostreams.IOStreams, checks []check, counts map[string]int) error {
	cs := ios.ColorScheme()
	if ios.IsStdoutTTY() {
		summary := "All checks were successful"
		switch {
		case counts["fail"] > 0 || counts["cancel"] > 0:
			summary = "Some checks were not successful"
		case counts["pending"] > 0:
			summary = "Some checks are still pending"
		}
		fmt.Fprintln(ios.Out, cs.Bold(summary))
		fmt.Fprintf(ios.Out, "%d cancelled, %d failing, %d successful, %d skipped, and %d pending checks\n\n",
			counts["cancel"], counts["fail"], counts["pass"], counts["skipping"], counts["pending"])
	}
	t := cmdutil.NewTable(ios)
	t.Header("", "Name", "Description", "Elapsed", "URL")
	for _, ch := range checks {
		elapsed := ""
		if !ch.StartedAt.IsZero() && !ch.CompletedAt.IsZero() && ch.CompletedAt.After(ch.StartedAt) {
			elapsed = ch.CompletedAt.Sub(ch.StartedAt).Round(time.Second).String()
		}
		if t.IsTTY() {
			mark, color := "✓", cs.Green
			switch ch.Bucket {
			case "fail":
				mark, color = "X", cs.Red
			case "cancel":
				mark, color = "-", cs.Gray
			case "pending":
				mark, color = "*", cs.Yellow
			case "skipping":
				mark, color = "-", cs.Gray
			}
			t.Field(mark, color, false)
			t.Field(ch.Name, nil, true)
			t.Field(ch.Description, cs.Gray, true)
			t.Field(elapsed, nil, false)
			t.Field(ch.Link, cs.Gray, true)
		} else {
			t.Field(ch.Name, nil, false)
			t.Field(ch.Bucket, nil, false)
			t.Field(elapsed, nil, false)
			t.Field(ch.Link, nil, false)
			t.Field(ch.Description, nil, false)
		}
		t.EndRow()
	}
	return t.Render()
}
