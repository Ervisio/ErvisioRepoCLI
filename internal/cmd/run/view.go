package run

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

func newView(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, jobFlag                        string
		attempt                                  int
		verbose, exitStatus, log, logFailed, web bool
		exporter                                 cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "view [<run-id>]",
		Short: "View a summary of a workflow run",
		Long: `View a summary of a workflow run.

Runs imported from GitHub show their summary and link only: their jobs,
logs and artifacts live on GitHub.`,
		Example: `# Interactively select a run to view, optionally selecting a single job
$ eg run view

# View a specific run
$ eg run view 12345

# View a specific job within a run
$ eg run view --job 456789

# View the full log for a specific job
$ eg run view --log --job 456789

# Exit non-zero if a run failed
$ eg run view 0451 --exit-status && echo "run pending or passed"`,
		Flags: cmdutil.NewFlagSet("view"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.BoolVarP(&verbose, "verbose", "v", false, "Show job steps")
	fl.BoolVarP(&exitStatus, "exit-status", "", false, "Exit with non-zero status if run failed")
	fl.StringVarP(&jobFlag, "job", "j", "", "View a specific job ID from a run")
	fl.BoolVarP(&log, "log", "", false, "View full log for either a run or specific job")
	fl.BoolVarP(&logFailed, "log-failed", "", false, "View the log for any failed steps in a run or specific job")
	fl.BoolVarP(&web, "web", "w", false, "Open run in the browser")
	fl.IntVarP(&attempt, "attempt", "a", 0, "The attempt number of the workflow run")
	cmdutil.AddJSONFlags(c, &exporter, append(append([]string(nil), runJSONFields...), "jobs"))
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		if log && logFailed {
			return cmdutil.FlagErrorf("specify only one of `--log` or `--log-failed`")
		}
		if web && (log || logFailed) {
			return cmdutil.FlagErrorf("specify only one of `--web` or `--log`")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var (
			runID int64
			job   *Job
		)
		if jobFlag != "" {
			jid, err := strconv.ParseInt(jobFlag, 10, 64)
			if err != nil {
				return cmdutil.FlagErrorf("invalid job ID %q", jobFlag)
			}
			var j Job
			if _, err := client.REST(http.MethodGet, ActionsPath(r)+"/jobs/"+jobFlag, nil, &j); err != nil {
				if api.IsNotFound(err) {
					return fmt.Errorf("could not find job %d", jid)
				}
				return err
			}
			job, runID = &j, j.RunID
		} else if len(args) > 0 {
			if runID, err = parseRunID(args[0]); err != nil {
				return err
			}
		} else if runID, err = pickRun(f, client, r, nil, "found no recent runs"); err != nil {
			return err
		}
		run, err := FetchRun(client, r, runID, attempt)
		if err != nil {
			return err
		}
		if web {
			u := run.HTMLURL
			if job != nil && job.HTMLURL != "" {
				u = job.HTMLURL
			}
			return f.OpenURL(u)
		}
		if run.External {
			if log || logFailed {
				return externalNote(run, "logs")
			}
			if exporter.Enabled() {
				m := runExport(run)
				m["jobs"] = []any{}
				return exporter.Write(f.IO, exporter.Pick(m))
			}
			printExternal(f.IO, run)
			if exitStatus && run.Failed() {
				return cmdutil.ErrSilent
			}
			return nil
		}
		var jobs []Job
		if job != nil {
			jobs = []Job{*job}
		} else if jobs, err = FetchJobs(client, r, run.ID, attempt); err != nil {
			return err
		}
		if exporter.Enabled() {
			m := runExport(run)
			m["jobs"] = jobsExport(jobs)
			if err := exporter.Write(f.IO, exporter.Pick(m)); err != nil {
				return err
			}
			if exitStatus && run.Failed() {
				return cmdutil.ErrSilent
			}
			return nil
		}
		if log || logFailed {
			if !run.Completed() {
				return fmt.Errorf("run %d is still in progress; logs will be available when it is complete", run.ID)
			}
			if job != nil && job.Status != "completed" {
				return fmt.Errorf("job %d is still in progress; logs will be available when it is complete", job.ID)
			}
			if err := printLogs(f, client, r, run, jobs, logFailed); err != nil {
				return err
			}
			if exitStatus && run.Failed() {
				return cmdutil.ErrSilent
			}
			return nil
		}
		var artifacts []Artifact
		if job == nil {
			if artifacts, err = listArtifacts(client, r, run.ID); err != nil {
				return err
			}
		}
		printRun(f.IO, run, jobs, artifacts, verbose || job != nil, job != nil)
		if exitStatus && run.Failed() {
			return cmdutil.ErrSilent
		}
		return nil
	}
	return c
}

func printExternal(ios *iostreams.IOStreams, run *Run) {
	cs := ios.ColorScheme()
	out := ios.Out
	fmt.Fprintf(out, "%s %s %s · %d\n", Symbol(cs, run.Status, run.Conc()), cs.Bold(run.Branch()), cs.Bold(run.Title()), run.ID)
	fmt.Fprintf(out, "Triggered via %s %s\n\n", run.Event, cmdutil.FuzzyAgo(time.Now(), parseTime(run.CreatedAt)))
	status := run.Status
	if run.Completed() {
		status = run.Conc()
	}
	fmt.Fprintf(out, "Workflow %s (%s): %s\n\n", run.Name, run.Path, status)
	fmt.Fprintln(out, "This run was imported from GitHub: its jobs, logs and artifacts live on GitHub.")
	fmt.Fprintf(out, "View this run on GitHub: %s\n", run.HTMLURL)
}

// printRun renders gh's run summary.
func printRun(ios *iostreams.IOStreams, run *Run, jobs []Job, artifacts []Artifact, steps, single bool) {
	cs := ios.ColorScheme()
	out := ios.Out
	now := time.Now()
	title := fmt.Sprintf("%s %s %s · %d", Symbol(cs, run.Status, run.Conc()), cs.Bold(run.Branch()), cs.Bold(run.Title()), run.ID)
	if run.RunAttempt > 1 {
		title += fmt.Sprintf(" (Attempt #%d)", run.RunAttempt)
	}
	fmt.Fprintln(out, title)
	fmt.Fprintf(out, "Triggered via %s %s\n", run.Event, cmdutil.FuzzyAgo(now, parseTime(run.CreatedAt)))
	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Bold("JOBS"))
	if len(jobs) == 0 {
		fmt.Fprintln(out, cs.Gray("No jobs"))
	}
	for i := range jobs {
		j := &jobs[i]
		line := fmt.Sprintf("%s %s", Symbol(cs, j.Status, deref(j.Conclusion)), cs.Bold(j.Name))
		if j.Status == "completed" || j.StartedAt != nil {
			line += " in " + FormatDuration(jobDuration(j))
		}
		fmt.Fprintf(out, "%s (ID %d)\n", line, j.ID)
		if steps || j.Status != "completed" || isFailure(deref(j.Conclusion)) && !single {
			for _, s := range j.Steps {
				if !steps && j.Status == "completed" && !isFailure(deref(s.Conclusion)) {
					continue
				}
				fmt.Fprintf(out, "  %s %s\n", Symbol(cs, s.Status, deref(s.Conclusion)), s.Name)
			}
		}
	}
	if len(artifacts) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("ARTIFACTS"))
		for _, a := range artifacts {
			if a.Expired {
				fmt.Fprintf(out, "%s %s\n", cs.Gray(a.Name), cs.Gray("(expired)"))
			} else {
				fmt.Fprintln(out, a.Name)
			}
		}
	}
	fmt.Fprintln(out)
	if single && len(jobs) == 1 {
		fmt.Fprintf(out, "To see the full job log, try: eg run view --log --job=%d\n", jobs[0].ID)
	} else if run.Failed() {
		fmt.Fprintf(out, "To see what failed, try: eg run view %d --log-failed\n", run.ID)
	} else if len(jobs) > 0 {
		fmt.Fprintf(out, "For more information about a job, try: eg run view --job=<job-id>\n")
	}
	fmt.Fprintf(out, "View this run on ErvisioRepo: %s\n", run.HTMLURL)
}

func isFailure(conclusion string) bool {
	switch conclusion {
	case "failure", "timed_out", "startup_failure":
		return true
	}
	return false
}

func listArtifacts(c *api.Client, r ghrepo.Repo, runID int64) ([]Artifact, error) {
	p := ActionsPath(r) + "/artifacts?per_page=100"
	if runID != 0 {
		p = ActionsPath(r) + "/runs/" + strconv.FormatInt(runID, 10) + "/artifacts?per_page=100"
	}
	var out []Artifact
	for next := p; next != ""; {
		var page struct {
			Artifacts []Artifact `json:"artifacts"`
		}
		var err error
		if next, err = c.REST(http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Artifacts...)
	}
	return out, nil
}

// zipName mirrors the server's escaping of job and step names in the log
// archive.
func zipName(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_", "\x00", "").Replace(strings.TrimSpace(s))
}

// printLogs downloads the log archive of the run and prints the step
// logs as gh does: "<job>\t<step>\t<line>".
func printLogs(f *cmdutil.Factory, c *api.Client, r ghrepo.Repo, run *Run, jobs []Job, failedOnly bool) error {
	resp, err := c.Request(http.MethodGet, c.RESTURL(ActionsPath(r)+"/runs/"+strconv.FormatInt(run.ID, 10)+"/logs"), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 300 {
		return api.HandleHTTPError(resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("download run logs: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("read run log archive: %w", err)
	}
	files := map[string]*zip.File{}
	for _, zf := range zr.File {
		files[zf.Name] = zf
	}
	if err := f.StartPager(); err != nil {
		fmt.Fprintf(f.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer f.StopPager()
	w := bufio.NewWriter(f.IO.Out)
	defer w.Flush() //nolint:errcheck // best effort on exit
	printed := false
	for i := range jobs {
		j := &jobs[i]
		if failedOnly && !isFailure(deref(j.Conclusion)) {
			continue
		}
		dir := zipName(j.Name)
		steps := append([]Step(nil), j.Steps...)
		sort.Slice(steps, func(a, b int) bool { return steps[a].Number < steps[b].Number })
		found := false
		for _, s := range steps {
			if failedOnly && !isFailure(deref(s.Conclusion)) {
				continue
			}
			zf := files[path.Join(dir, strconv.Itoa(s.Number)+"_"+zipName(s.Name)+".txt")]
			if zf == nil {
				continue
			}
			found = true
			if err := copyPrefixed(w, zf, j.Name+"\t"+s.Name+"\t"); err != nil {
				return err
			}
			printed = true
		}
		if found || failedOnly {
			continue
		}
		// No step files: print the whole job log.
		for name, zf := range files {
			if !strings.Contains(name, "/") && strings.HasSuffix(name, "_"+dir+".txt") {
				if err := copyPrefixed(w, zf, j.Name+"\tUNKNOWN STEP\t"); err != nil {
					return err
				}
				printed = true
				break
			}
		}
	}
	if !printed && failedOnly {
		return errors.New("no failed steps found in the run logs")
	}
	return nil
}

func copyPrefixed(w *bufio.Writer, zf *zip.File, prefix string) error {
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", zf.Name, err)
	}
	defer rc.Close() //nolint:errcheck // read-only
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		if _, err := w.WriteString(prefix + sc.Text() + "\n"); err != nil {
			return fmt.Errorf("write log: %w", err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read %s: %w", zf.Name, err)
	}
	return nil
}
