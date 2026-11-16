// Package variable implements `eg variable`.
package variable

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// New returns the variable command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:   "variable <command>",
		Short: "Manage Ervisio Actions variables",
		Long: `Variables can be set at the repository level for use in Ervisio Actions.

Organization and environment variables are not available on an
ErvisioRepo server.`,
	}
	c.AddCommand(newList(f), newGet(f), newSet(f), newDelete(f))
	return c
}

type variable struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (v *variable) export() map[string]any {
	return map[string]any{"name": v.Name, "value": v.Value, "createdAt": v.CreatedAt, "updatedAt": v.UpdatedAt,
		"visibility": "", "numSelectedRepos": 0, "selectedReposURL": ""}
}

var variableJSONFields = []string{"createdAt", "name", "numSelectedRepos", "selectedReposURL", "updatedAt", "value", "visibility"}

func varsPath(r ghrepo.Repo) string { return shared.RepoPath(r) + "/actions/variables" }

type target struct {
	f        *cmdutil.Factory
	repoFlag string
	scope    shared.ScopeFlags
}

func (t *target) register(c *cmdutil.Command) {
	shared.AddRepoFlag(c, &t.repoFlag)
	t.scope.Register(c.Flags, false, false)
}

func (t *target) resolve() (ghrepo.Repo, *api.Client, error) {
	if err := t.scope.Check("variables"); err != nil {
		return ghrepo.Repo{}, nil, err
	}
	r, err := t.f.BaseRepo(t.repoFlag)
	if err != nil {
		return ghrepo.Repo{}, nil, err
	}
	client, err := t.f.Client(r.Host)
	return r, client, err
}

func newList(f *cmdutil.Factory) *cmdutil.Command {
	tg := &target{f: f}
	var exporter cmdutil.Exporter
	c := &cmdutil.Command{
		Use:     "list",
		Short:   "List variables",
		Long:    "List variables on a repository.",
		Aliases: []string{"ls"},
		Flags:   cmdutil.NewFlagSet("list"),
		Args:    cmdutil.NoArgs,
	}
	tg.register(c)
	cmdutil.AddJSONFlags(c, &exporter, variableJSONFields)
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		r, client, err := tg.resolve()
		if err != nil {
			return err
		}
		var list []variable
		for next := varsPath(r) + "?per_page=30"; next != ""; {
			var page struct {
				Variables []variable `json:"variables"`
			}
			if next, err = client.REST(http.MethodGet, next, nil, &page); err != nil {
				return err
			}
			list = append(list, page.Variables...)
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(list))
			for i := range list {
				out[i] = exporter.Pick(list[i].export())
			}
			return exporter.Write(f.IO, out)
		}
		if len(list) == 0 {
			if f.IO.IsStdoutTTY() {
				fmt.Fprintf(f.IO.ErrOut, "no variables found in %s\n", r.FullName())
			}
			return cmdutil.ErrSilent
		}
		now := time.Now()
		t := cmdutil.NewTable(f.IO)
		t.Header("Name", "Value", "Updated")
		for _, v := range list {
			t.Field(v.Name, nil, false)
			t.Field(v.Value, nil, true)
			if t.IsTTY() {
				ts, _ := time.Parse(time.RFC3339, v.UpdatedAt)
				t.Field(cmdutil.FuzzyAgo(now, ts), f.IO.ColorScheme().Gray, false)
			} else {
				t.Field(v.UpdatedAt, nil, false)
			}
			t.EndRow()
		}
		return t.Render()
	}
	return c
}

func fetch(c *api.Client, r ghrepo.Repo, name string) (*variable, error) {
	var v variable
	if _, err := c.REST(http.MethodGet, varsPath(r)+"/"+url.PathEscape(name), nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func newGet(f *cmdutil.Factory) *cmdutil.Command {
	tg := &target{f: f}
	var exporter cmdutil.Exporter
	c := &cmdutil.Command{
		Use:   "get <variable-name>",
		Short: "Get variables",
		Long:  "Get a variable on a repository.",
		Example: `# Get the value of a variable on the current repository
$ eg variable get MYVARIABLE`,
		Flags: cmdutil.NewFlagSet("get"),
		Args:  cmdutil.ExactArgs(1, "must pass a single variable name"),
	}
	tg.register(c)
	cmdutil.AddJSONFlags(c, &exporter, variableJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		r, client, err := tg.resolve()
		if err != nil {
			return err
		}
		v, err := fetch(client, r, args[0])
		if err != nil {
			if api.IsNotFound(err) {
				return fmt.Errorf("variable %s was not found", args[0])
			}
			return fmt.Errorf("failed to get variable %s: %w", args[0], err)
		}
		if exporter.Enabled() {
			return exporter.Write(f.IO, exporter.Pick(v.export()))
		}
		fmt.Fprintln(f.IO.Out, v.Value)
		return nil
	}
	return c
}

func newSet(f *cmdutil.Factory) *cmdutil.Command {
	tg := &target{f: f}
	var (
		body, envFile, visibility string
		repos                     []string
	)
	c := &cmdutil.Command{
		Use:   "set <variable-name>",
		Short: "Create or update variables",
		Long: `Set a value for a variable on a repository.

The value is read from --body, else from standard input when it is not a
terminal, else it is asked for. Multiple variables can be set from a dotenv
file with --env-file.`,
		Example: `# Add variable value for the current repository in an interactive prompt
$ eg variable set MYVARIABLE

# Read variable value from an environment variable
$ eg variable set MYVARIABLE --body "$ENV_VALUE"

# Read variable value from a file
$ eg variable set MYVARIABLE < myfile.txt

# Set multiple variables imported from the ".env" file
$ eg variable set -f .env`,
		Flags: cmdutil.NewFlagSet("set"),
		Args:  cmdutil.MaxArgs(1),
	}
	tg.register(c)
	fl := c.Flags
	fl.StringVarP(&body, "body", "b", "", "The value for the variable (reads from standard input if not specified)")
	fl.StringVarP(&envFile, "env-file", "f", "", "Load variable names and values from a dotenv-formatted `file`")
	fl.StringVarP(&visibility, "visibility", "v", "", "Set visibility for an organization variable (not supported)")
	fl.StringArrayVarP(&repos, "repos", "r", "List of `repositories` that can access an organization variable (not supported)")
	c.Run = func(c *cmdutil.Command, args []string) error {
		if visibility != "" || len(repos) > 0 {
			return cmdutil.Unsupported("Variable visibility and selected repositories", "organization variables")
		}
		if len(args) == 0 && envFile == "" {
			return cmdutil.FlagErrorf("must pass name argument")
		}
		if len(args) > 0 && envFile != "" {
			return cmdutil.FlagErrorf("specify only one of `<variable-name>` or `--env-file`")
		}
		values := map[string]string{}
		var order []string
		if envFile != "" {
			in := f.IO.In
			if envFile != "-" {
				fh, err := os.Open(envFile)
				if err != nil {
					return fmt.Errorf("failed to open env file: %w", err)
				}
				defer fh.Close() //nolint:errcheck // read-only
				in = fh
			}
			keys, vals, err := shared.ParseDotenv(in)
			if err != nil {
				return fmt.Errorf("error parsing env file: %w", err)
			}
			if len(keys) == 0 {
				return errors.New("no variables found in file")
			}
			values, order = vals, keys
		} else {
			if !c.Flags.Changed("body") {
				if f.IO.CanPrompt() {
					v, err := (&cmdutil.Prompter{IO: f.IO}).Input("Paste your variable", "")
					if err != nil {
						return err
					}
					body = v
				} else {
					data, err := io.ReadAll(f.IO.In)
					if err != nil {
						return fmt.Errorf("failed to read variable from standard input: %w", err)
					}
					body = strings.TrimRight(string(data), "\r\n")
				}
			}
			values[args[0]], order = body, []string{args[0]}
		}
		r, client, err := tg.resolve()
		if err != nil {
			return err
		}
		ios := f.IO
		for _, name := range order {
			verb := "Created"
			_, err := client.REST(http.MethodPost, varsPath(r), map[string]any{"name": name, "value": values[name]}, nil)
			if api.IsStatus(err, http.StatusConflict) {
				verb = "Updated"
				_, err = client.REST(http.MethodPatch, varsPath(r)+"/"+url.PathEscape(name), map[string]any{"name": name, "value": values[name]}, nil)
			}
			if err != nil {
				return fmt.Errorf("failed to set variable %q: %w", name, err)
			}
			if ios.IsStdoutTTY() {
				fmt.Fprintf(ios.Out, "%s %s variable %s for %s\n", ios.ColorScheme().SuccessIcon(), verb, name, r.FullName())
			}
		}
		return nil
	}
	return c
}

func newDelete(f *cmdutil.Factory) *cmdutil.Command {
	tg := &target{f: f}
	c := &cmdutil.Command{
		Use:     "delete <variable-name>",
		Short:   "Delete variables",
		Long:    "Delete a variable on a repository.",
		Aliases: []string{"remove"},
		Flags:   cmdutil.NewFlagSet("delete"),
		Args:    cmdutil.ExactArgs(1, "must pass name argument"),
	}
	tg.register(c)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, client, err := tg.resolve()
		if err != nil {
			return err
		}
		if _, err := client.REST(http.MethodDelete, varsPath(r)+"/"+url.PathEscape(args[0]), nil, nil); err != nil {
			if api.IsNotFound(err) {
				return fmt.Errorf("failed to delete variable %s: variable not found in %s", args[0], r.FullName())
			}
			return fmt.Errorf("failed to delete variable %s: %w", args[0], err)
		}
		if f.IO.IsStdoutTTY() {
			fmt.Fprintf(f.IO.Out, "%s Deleted variable %s from %s\n", f.IO.ColorScheme().Red("✓"), args[0], r.FullName())
		}
		return nil
	}
	return c
}
