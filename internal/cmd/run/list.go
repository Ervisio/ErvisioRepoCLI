package run

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

var runStatuses = []string{"queued", "completed", "in_progress", "requested", "waiting", "pending",
	"action_required", "cancelled", "failure", "neutral", "skipped", "stale", "startup_failure", "success", "timed_out"}

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, workflow, branch, user, event, status, created, commit string
		limit                                                            int
		all                                                              bool
		exporter                                                         cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "list",
		Short:   "List recent workflow runs",
		Aliases: []string{"ls"},
		Long: `List recent workflow runs.

Runs imported from GitHub are listed too; their URL points to GitHub.`,
		Flags: cmdutil.NewFlagSet("list"),
		Args:  cmdutil.NoArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.IntVarP(&limit, "limit", "L", 20, "Maximum number of runs to fetch")
	fl.StringVarP(&workflow, "workflow", "w", "", "Filter runs by workflow")
	fl.StringVarP(&branch, "branch", "b", "", "Filter runs by branch")
	fl.StringVarP(&user, "user", "u", "", "Filter runs by user who triggered the run")
	fl.StringVarP(&event, "event", "e", "", "Filter runs by which `event` triggered the run")
	fl.EnumVarP(&status, "status", "s", "", runStatuses, "Filter runs by status")
	fl.StringVarP(&created, "created", "", "", "Filter runs by the `date` it was created")
	fl.StringVarP(&commit, "commit", "c", "", "Filter runs by the `SHA` of the commit")
	fl.BoolVarP(&all, "all", "a", false, "Include disabled workflows")
	cmdutil.AddJSONFlags(c, &exporter, runJSONFields)
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		if limit < 1 {
			return cmdutil.FlagErrorf("invalid limit: %v", limit)
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		flt := Filter{Branch: branch, Actor: user, Event: event, Status: status, Created: created, Commit: commit}
		if workflow != "" {
			w, err := FindWorkflow(f, client, r, workflow)
			if err != nil {
				return err
			}
			flt.WorkflowID = w.ID
		}
		runs, _, err := ListRuns(client, r, flt, limit)
		if err != nil {
			return err
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(runs))
			for i := range runs {
				out[i] = exporter.Pick(runExport(&runs[i]))
			}
			return exporter.Write(f.IO, out)
		}
		ios := f.IO
		if len(runs) == 0 {
			if ios.IsStdoutTTY() {
				fmt.Fprintln(ios.ErrOut, "no runs found")
			}
			return cmdutil.ErrSilent
		}
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(ios.ErrOut, "error starting pager: %v\n", err)
		}
		defer f.StopPager()
		cs := ios.ColorScheme()
		now := time.Now()
		t := cmdutil.NewTable(ios)
		if t.IsTTY() {
			t.Header("", "Title", "Workflow", "Branch", "Event", "ID", "Elapsed", "Age")
		} else {
			t.Header("Status", "Conclusion", "Title", "Workflow", "Branch", "Event", "ID", "Elapsed", "Age")
		}
		for i := range runs {
			run := &runs[i]
			if t.IsTTY() {
				t.Field(Symbol(cs, run.Status, run.Conc()), nil, false)
			} else {
				t.Field(run.Status, nil, false)
				t.Field(run.Conc(), nil, false)
			}
			t.Field(run.Title(), cs.Bold, true)
			t.Field(run.Name, nil, true)
			t.Field(run.Branch(), cs.Cyan, true)
			t.Field(run.Event, nil, false)
			t.Field(strconv.FormatInt(run.ID, 10), cs.Cyan, false)
			t.Field(FormatDuration(run.Elapsed(now)), nil, false)
			if t.IsTTY() {
				t.Field(cmdutil.FuzzyAgo(now, parseTime(run.CreatedAt)), cs.Gray, false)
			} else {
				t.Field(run.CreatedAt, nil, false)
			}
			t.EndRow()
		}
		return t.Render()
	}
	return c
}
