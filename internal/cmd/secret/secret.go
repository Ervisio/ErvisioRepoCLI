// Package secret implements `eg secret`.
package secret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/nacl/box"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// New returns the secret command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:   "secret <command>",
		Short: "Manage Ervisio Actions secrets",
		Long: `Secrets can be set at the repository level for use in Ervisio Actions.

Organization, environment and user secrets are not available on an
ErvisioRepo server.`,
	}
	c.AddCommand(newList(f), newSet(f), newDelete(f))
	return c
}

func secretsPath(r ghrepo.Repo) string { return shared.RepoPath(r) + "/actions/secrets" }

var secretJSONFields = []string{"name", "numSelectedRepos", "selectedReposURL", "updatedAt", "visibility"}

type secret struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag string
		scope    shared.ScopeFlags
		exporter cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "list",
		Short:   "List secrets",
		Long:    "List secrets on a repository.",
		Aliases: []string{"ls"},
		Flags:   cmdutil.NewFlagSet("list"),
		Args:    cmdutil.NoArgs,
	}
	shared.AddRepoFlag(c, &repoFlag)
	scope.Register(c.Flags, true, true)
	cmdutil.AddJSONFlags(c, &exporter, secretJSONFields)
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		if err := scope.Check("secrets"); err != nil {
			return err
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var list []secret
		for next := secretsPath(r) + "?per_page=100"; next != ""; {
			var page struct {
				Secrets []secret `json:"secrets"`
			}
			if next, err = client.REST(http.MethodGet, next, nil, &page); err != nil {
				return err
			}
			list = append(list, page.Secrets...)
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(list))
			for i, s := range list {
				out[i] = exporter.Pick(map[string]any{"name": s.Name, "updatedAt": s.UpdatedAt, "visibility": "",
					"numSelectedRepos": 0, "selectedReposURL": ""})
			}
			return exporter.Write(f.IO, out)
		}
		if len(list) == 0 {
			if f.IO.IsStdoutTTY() {
				fmt.Fprintf(f.IO.ErrOut, "no secrets found in %s\n", r.FullName())
			}
			return cmdutil.ErrSilent
		}
		now := time.Now()
		t := cmdutil.NewTable(f.IO)
		t.Header("Name", "Updated")
		for _, s := range list {
			t.Field(s.Name, nil, false)
			if t.IsTTY() {
				ts, _ := time.Parse(time.RFC3339, s.UpdatedAt)
				t.Field(cmdutil.FuzzyAgo(now, ts), f.IO.ColorScheme().Gray, false)
			} else {
				t.Field(s.UpdatedAt, nil, false)
			}
			t.EndRow()
		}
		return t.Render()
	}
	return c
}

func newSet(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, body, envFile string
		noStore                 bool
		scope                   shared.ScopeFlags
		visibility              string
		repos                   []string
	)
	c := &cmdutil.Command{
		Use:   "set <secret-name>",
		Short: "Create or update secrets",
		Long: `Set a value for a secret on a repository.

The secret value is read from --body, else from standard input when it is
not a terminal, else it is asked for. Multiple secrets can be set from a
dotenv file with --env-file.

The value is encrypted on this machine with the public key of the
repository (a libsodium sealed box) before it is sent.`,
		Example: `# Paste secret value for the current repository in an interactive prompt
$ eg secret set MYSECRET

# Read secret value from an environment variable
$ eg secret set MYSECRET --body "$ENV_VALUE"

# Read secret value from a file
$ eg secret set MYSECRET < myfile.txt

# Set multiple secrets imported from the ".env" file
$ eg secret set -f .env`,
		Flags: cmdutil.NewFlagSet("set"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	scope.Register(fl, true, true)
	fl.StringVarP(&body, "body", "b", "", "The value for the secret (reads from standard input if not specified)")
	fl.StringVarP(&envFile, "env-file", "f", "", "Load secret names and values from a dotenv-formatted `file`")
	fl.BoolVarP(&noStore, "no-store", "", false, "Print the encrypted, base64-encoded value instead of storing it on ErvisioRepo")
	fl.StringVarP(&visibility, "visibility", "v", "", "Set visibility for an organization secret (not supported)")
	fl.StringArrayVarP(&repos, "repos", "r", "List of `repositories` that can access an organization or user secret (not supported)")
	c.Run = func(c *cmdutil.Command, args []string) error {
		if err := scope.Check("secrets"); err != nil {
			return err
		}
		if visibility != "" || len(repos) > 0 {
			return cmdutil.Unsupported("Secret visibility and selected repositories", "organization secrets")
		}
		if len(args) == 0 && envFile == "" {
			return cmdutil.FlagErrorf("must pass name argument")
		}
		if len(args) > 0 && envFile != "" {
			return cmdutil.FlagErrorf("specify only one of `<secret-name>` or `--env-file`")
		}
		if envFile != "" && c.Flags.Changed("body") {
			return cmdutil.FlagErrorf("specify only one of `--body` or `--env-file`")
		}
		values, order, err := secretValues(f, args, body, c.Flags.Changed("body"), envFile)
		if err != nil {
			return err
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var key struct {
			KeyID string `json:"key_id"`
			Key   string `json:"key"`
		}
		if _, err := client.REST(http.MethodGet, secretsPath(r)+"/public-key", nil, &key); err != nil {
			if api.IsStatus(err, http.StatusServiceUnavailable) {
				return fmt.Errorf("the server has no secrets key configured: %w", err)
			}
			return fmt.Errorf("failed to fetch public key: %w", err)
		}
		pub, err := base64.StdEncoding.DecodeString(key.Key)
		if err != nil || len(pub) != 32 {
			return errors.New("the server returned an invalid public key")
		}
		var pk [32]byte
		copy(pk[:], pub)
		ios := f.IO
		cs := ios.ColorScheme()
		for _, name := range order {
			sealed, err := box.SealAnonymous(nil, []byte(values[name]), &pk, rand.Reader)
			if err != nil {
				return fmt.Errorf("failed to encrypt secret: %w", err)
			}
			enc := base64.StdEncoding.EncodeToString(sealed)
			if noStore {
				fmt.Fprintln(ios.Out, enc)
				continue
			}
			req := map[string]any{"encrypted_value": enc, "key_id": key.KeyID}
			if _, err := client.REST(http.MethodPut, secretsPath(r)+"/"+url.PathEscape(name), req, nil); err != nil {
				return fmt.Errorf("failed to set secret %q: %w", name, err)
			}
			if ios.IsStdoutTTY() {
				fmt.Fprintf(ios.Out, "%s Set Actions secret %s for %s\n", cs.SuccessIcon(), name, r.FullName())
			}
		}
		return nil
	}
	return c
}

// secretValues collects the name/value pairs to set.
func secretValues(f *cmdutil.Factory, args []string, body string, hasBody bool, envFile string) (map[string]string, []string, error) {
	if envFile != "" {
		in := f.IO.In
		if envFile != "-" {
			fh, err := os.Open(envFile)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to open env file: %w", err)
			}
			defer fh.Close() //nolint:errcheck // read-only
			in = fh
		}
		keys, values, err := shared.ParseDotenv(in)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing env file: %w", err)
		}
		if len(keys) == 0 {
			return nil, nil, errors.New("no secrets found in file")
		}
		return values, keys, nil
	}
	name := args[0]
	if !hasBody {
		ios := f.IO
		if ios.CanPrompt() {
			v, err := (&cmdutil.Prompter{IO: ios}).Password("Paste your secret:")
			if err != nil {
				return nil, nil, err
			}
			body = v
		} else {
			data, err := io.ReadAll(ios.In)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read secret from standard input: %w", err)
			}
			body = strings.TrimRight(string(data), "\r\n")
		}
	}
	return map[string]string{name: body}, []string{name}, nil
}

func newDelete(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag string
		scope    shared.ScopeFlags
	)
	c := &cmdutil.Command{
		Use:     "delete <secret-name>",
		Short:   "Delete secrets",
		Long:    "Delete a secret on a repository.",
		Aliases: []string{"remove"},
		Flags:   cmdutil.NewFlagSet("delete"),
		Args:    cmdutil.ExactArgs(1, "must pass name argument"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	scope.Register(c.Flags, true, true)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := scope.Check("secrets"); err != nil {
			return err
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if _, err := client.REST(http.MethodDelete, secretsPath(r)+"/"+url.PathEscape(args[0]), nil, nil); err != nil {
			if api.IsNotFound(err) {
				return fmt.Errorf("failed to delete secret %s: secret not found in %s", args[0], r.FullName())
			}
			return fmt.Errorf("failed to delete secret %s: %w", args[0], err)
		}
		if f.IO.IsStdoutTTY() {
			fmt.Fprintf(f.IO.Out, "%s Deleted Actions secret %s from %s\n", f.IO.ColorScheme().Red("✓"), args[0], r.FullName())
		}
		return nil
	}
	return c
}
