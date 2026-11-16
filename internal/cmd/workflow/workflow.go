// Package workflow implements `eg workflow`.
package workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/run"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// New returns the workflow command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "workflow <command>",
		Short:   "View details about Ervisio Actions workflows",
		Long:    "List, view, and run workflows in Ervisio Actions.",
		GroupID: "actions",
	}
	c.AddCommand(newList(f), newView(f), newRun(f), newToggle("enable"), newToggle("disable"))
	return c
}

var workflowJSONFields = []string{"id", "name", "path", "state"}

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag string
		limit    int
		all      bool
		exporter cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "list",
		Short:   "List workflows",
		Long:    "List workflow files. Workflows cannot be disabled on ErvisioRepo, so --all changes nothing.",
		Aliases: []string{"ls"},
		Flags:   cmdutil.NewFlagSet("list"),
		Args:    cmdutil.NoArgs,
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.IntVarP(&limit, "limit", "L", 50, "Maximum number of workflows to fetch")
	c.Flags.BoolVarP(&all, "all", "a", false, "Include disabled workflows")
	cmdutil.AddJSONFlags(c, &exporter, workflowJSONFields)
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
		list, err := run.ListWorkflows(client, r)
		if err != nil {
			return err
		}
		if len(list) > limit {
			list = list[:limit]
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(list))
			for i, w := range list {
				out[i] = exporter.Pick(map[string]any{"id": w.ID, "name": w.Name, "path": w.Path, "state": w.State})
			}
			return exporter.Write(f.IO, out)
		}
		if len(list) == 0 {
			if f.IO.IsStdoutTTY() {
				fmt.Fprintln(f.IO.ErrOut, "no workflows found")
			}
			return cmdutil.ErrSilent
		}
		cs := f.IO.ColorScheme()
		t := cmdutil.NewTable(f.IO)
		t.Header("Name", "State", "ID")
		for _, w := range list {
			t.Field(w.Name, cs.Bold, true)
			t.Field(w.State, nil, false)
			t.Field(strconv.FormatInt(w.ID, 10), cs.Cyan, false)
			t.EndRow()
		}
		return t.Render()
	}
	return c
}

func newView(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, ref string
		web, yaml     bool
	)
	c := &cmdutil.Command{
		Use:   "view [<workflow-id> | <workflow-name> | <filename>]",
		Short: "View the summary of a workflow",
		Example: `# Interactively select a workflow to view
$ eg workflow view

# View a specific workflow
$ eg workflow view 0451`,
		Flags: cmdutil.NewFlagSet("view"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.BoolVarP(&web, "web", "w", false, "Open workflow in the browser")
	fl.BoolVarP(&yaml, "yaml", "y", false, "View the workflow yaml file")
	fl.StringVarP(&ref, "ref", "r", "", "The branch or tag name which contains the version of the workflow file you'd like to view")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if ref != "" && !yaml && !web {
			return cmdutil.FlagErrorf("`--yaml` or `--web` required when specifying `--ref`")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		w, err := selectWorkflow(f, client, r, args)
		if err != nil {
			return err
		}
		if web {
			u := w.HTMLURL
			if ref != "" {
				u = f.WebURL(r, "blob/"+url.PathEscape(ref)+"/"+w.Path)
			}
			return f.OpenURL(u)
		}
		ios := f.IO
		cs := ios.ColorScheme()
		if yaml {
			p := shared.RepoPath(r) + "/contents/" + w.Path
			if ref != "" {
				p += "?ref=" + url.QueryEscape(ref)
			}
			resp, err := client.Request(http.MethodGet, client.RESTURL(p), nil, http.Header{"Accept": {"application/vnd.github.raw"}})
			if err != nil {
				return err
			}
			defer resp.Body.Close() //nolint:errcheck // read-only body
			if resp.StatusCode >= 300 {
				if resp.StatusCode == http.StatusNotFound {
					return fmt.Errorf("could not find workflow file %s, try specifying a branch or tag using `--ref`", w.Path)
				}
				return api.HandleHTTPError(resp)
			}
			if ios.IsStdoutTTY() {
				fmt.Fprintf(ios.Out, "%s - %s\n", cs.Bold(w.Name), cs.Gray(w.Base()))
				fmt.Fprintf(ios.Out, "ID: %s\n\n", cs.Cyan(strconv.FormatInt(w.ID, 10)))
			}
			_, err = io.Copy(ios.Out, resp.Body)
			return err
		}
		runs, total, err := run.ListRuns(client, r, run.Filter{WorkflowID: w.ID}, 5)
		if err != nil {
			return err
		}
		out := ios.Out
		fmt.Fprintf(out, "%s - %s\n", cs.Bold(w.Name), cs.Gray(w.Base()))
		fmt.Fprintf(out, "ID: %s\n\n", cs.Cyan(strconv.FormatInt(w.ID, 10)))
		fmt.Fprintf(out, "Total runs %d\n", total)
		if len(runs) > 0 {
			fmt.Fprintln(out, "Recent runs")
			t := cmdutil.NewTable(ios)
			t.Header("", "Title", "Workflow", "Branch", "Event", "ID")
			for i := range runs {
				rn := &runs[i]
				if t.IsTTY() {
					t.Field(run.Symbol(cs, rn.Status, rn.Conc()), nil, false)
				} else {
					t.Field(rn.Status+" "+rn.Conc(), nil, false)
				}
				t.Field(rn.Title(), cs.Bold, true)
				t.Field(rn.Name, nil, true)
				t.Field(rn.Branch(), cs.Cyan, true)
				t.Field(rn.Event, nil, false)
				t.Field(strconv.FormatInt(rn.ID, 10), cs.Cyan, false)
				t.EndRow()
			}
			if err := t.Render(); err != nil {
				return err
			}
		}
		fmt.Fprintln(out)
		fmt.Fprintf(out, "To see more runs for this workflow, try: eg run list --workflow %s\n", w.Base())
		fmt.Fprintf(out, "To see the YAML for this workflow, try: eg workflow view %s --yaml\n", w.Base())
		return nil
	}
	return c
}

// selectWorkflow resolves the argument, or prompts for a workflow.
func selectWorkflow(f *cmdutil.Factory, c *api.Client, r ghrepo.Repo, args []string) (*run.Workflow, error) {
	if len(args) > 0 {
		return run.FindWorkflow(f, c, r, args[0])
	}
	if !f.IO.CanPrompt() {
		return nil, cmdutil.FlagErrorf("workflow ID, name, or filename required when not running interactively")
	}
	list, err := run.ListWorkflows(c, r)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("could not find any workflows in %s", r.FullName())
	}
	opts := make([]string, len(list))
	for i, w := range list {
		opts[i] = fmt.Sprintf("%s (%s)", w.Name, w.Base())
	}
	i, err := (&cmdutil.Prompter{IO: f.IO}).Select("Select a workflow", 0, opts)
	if err != nil {
		return nil, err
	}
	return &list[i], nil
}

func newRun(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, ref string
		raw, typed    []string
		jsonIn        bool
	)
	c := &cmdutil.Command{
		Use:   "run [<workflow-id> | <workflow-name> | <filename>]",
		Short: "Run a workflow by creating a workflow_dispatch event",
		Long: `Create a workflow_dispatch event for a given workflow.

This command will trigger a workflow run if the workflow file contains a
workflow_dispatch trigger.

Workflow inputs are given with -f/--raw-field and -F/--field (a value
starting with "@" is read from that file, "@-" from standard input), or as
a JSON object on standard input with --json.`,
		Example: `# Run the workflow file 'triage.yml' at the remote's default branch
$ eg workflow run triage.yml

# Run the workflow file 'triage.yml' at a specified ref
$ eg workflow run triage.yml --ref my-branch

# Run the workflow file 'triage.yml' with command line inputs
$ eg workflow run triage.yml -f name=scully -f greeting=hello

# Run the workflow file 'triage.yml' with JSON via standard input
$ echo '{"name":"scully", "greeting":"hello"}' | eg workflow run triage.yml --json`,
		Flags: cmdutil.NewFlagSet("run"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.StringVarP(&ref, "ref", "r", "", "Branch or tag name which contains the version of the workflow file you'd like to run")
	fl.StringArrayVarP(&raw, "raw-field", "f", "Add a string parameter in `key=value` format")
	fl.StringArrayVarP(&typed, "field", "F", "Add a string parameter in `key=value` format, respecting @ syntax")
	fl.BoolVarP(&jsonIn, "json", "", false, "Read workflow inputs as JSON via STDIN")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if jsonIn && (len(raw) > 0 || len(typed) > 0) {
			return cmdutil.FlagErrorf("only one of STDIN or -f/-F can be passed")
		}
		if len(args) == 0 && !f.IO.CanPrompt() {
			return cmdutil.FlagErrorf("workflow ID, name, or filename required when not running interactively")
		}
		inputs := map[string]any{}
		if jsonIn {
			data, err := io.ReadAll(f.IO.In)
			if err != nil {
				return fmt.Errorf("failed to read from STDIN: %w", err)
			}
			if err := json.Unmarshal(data, &inputs); err != nil {
				return fmt.Errorf("could not parse provided JSON: %w", err)
			}
		}
		for _, kv := range raw {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return cmdutil.FlagErrorf("field %q requires a value separated by an '=' sign", kv)
			}
			inputs[k] = v
		}
		for _, kv := range typed {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return cmdutil.FlagErrorf("field %q requires a value separated by an '=' sign", kv)
			}
			if strings.HasPrefix(v, "@") {
				data, err := readValueFile(f, v[1:])
				if err != nil {
					return err
				}
				v = data
			}
			inputs[k] = v
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		w, err := selectWorkflow(f, client, r, args)
		if err != nil {
			return err
		}
		body := map[string]any{"inputs": inputs}
		if ref != "" {
			body["ref"] = ref
		}
		if _, err := client.REST(http.MethodPost, run.ActionsPath(r)+"/workflows/"+strconv.FormatInt(w.ID, 10)+"/dispatches", body, nil); err != nil {
			if api.IsStatus(err, http.StatusUnprocessableEntity) {
				return fmt.Errorf("could not create workflow dispatch event: %w", err)
			}
			return err
		}
		ios := f.IO
		if ios.IsStdoutTTY() {
			cs := ios.ColorScheme()
			at := ref
			if at == "" {
				at = "the default branch"
			}
			fmt.Fprintf(ios.Out, "%s Created workflow_dispatch event for %s at %s\n\n", cs.SuccessIcon(), cs.Cyan(w.Base()), cs.Bold(at))
			fmt.Fprintf(ios.Out, "To see runs for this workflow, try: eg run list --workflow=%s\n", w.Base())
		}
		return nil
	}
	return c
}

func readValueFile(f *cmdutil.Factory, name string) (string, error) {
	var (
		data []byte
		err  error
	)
	if name == "-" {
		data, err = io.ReadAll(f.IO.In)
	} else {
		data, err = os.ReadFile(name)
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	return string(data), nil
}

// newToggle builds enable and disable: workflows cannot be disabled on
// an ErvisioRepo server.
func newToggle(verb string) *cmdutil.Command {
	var repoFlag string
	c := &cmdutil.Command{
		Use:   verb + " [<workflow-id> | <workflow-name> | <filename>]",
		Short: strings.ToUpper(verb[:1]) + verb[1:] + " a workflow",
		Long: strings.ToUpper(verb[:1]) + verb[1:] + ` a workflow.

Not supported: workflows are always active on an ErvisioRepo server.`,
		Flags: cmdutil.NewFlagSet(verb),
		Args:  cmdutil.MaxArgs(1),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Run = func(*cmdutil.Command, []string) error {
		return cmdutil.Unsupported(strings.ToUpper(verb[:1])+strings.TrimSuffix(verb[1:], "e")+"ing workflows", "PUT /repos/{owner}/{repo}/actions/workflows/{id}/"+verb)
	}
	return c
}
