package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
)

type loginOptions struct {
	hostname    string
	gitProtocol string
	withToken   bool
	web         bool
	scopes      []string
	insecure    bool
}

func newLogin(f *cmdutil.Factory) *cmdutil.Command {
	o := &loginOptions{}
	c := &cmdutil.Command{
		Use:   "login",
		Short: "Log in to an ErvisioRepo server",
		Long: `Authenticate with an ErvisioRepo server.

The default authentication mode is a web-based browser flow (OAuth device
flow): eg shows a one-time code and the URL of the server's device page;
open it in a browser, sign in and enter the code. eg polls the server
until the login is approved. The interactive prompt also offers to paste a
personal access token instead, created under Settings → Tokens with at
least the repo:write and user:read scopes.

The minimum scopes requested by the browser flow are "repo", "read:org"
and "workflow"; add more with --scopes.

Alternatively, use --with-token to pass in a token on standard input.

The token is stored in plain text in hosts.yml inside the eg configuration
directory (file mode 0600).

Alternatively, eg will use the authentication token found in the EG_TOKEN
environment variable. This method is most suitable for "headless" use of
eg such as in automation.`,
		Example: `# Start interactive setup
$ eg auth login

# Authenticate against a specific server
$ eg auth login --hostname code.example.com

# Use a local server reached over plain http
$ eg auth login --hostname http://localhost:8080

# Use a token from a file
$ eg auth login --hostname code.example.com --with-token < mytoken.txt`,
		Flags: cmdutil.NewFlagSet("login"),
		Args:  cmdutil.NoArgs,
	}
	c.Flags.StringVarP(&o.hostname, "hostname", "h", "", "The `hostname` of the ErvisioRepo server to authenticate with")
	c.Flags.BoolVarP(&o.withToken, "with-token", "", false, "Read token from standard input")
	c.Flags.EnumVarP(&o.gitProtocol, "git-protocol", "p", "", []string{"ssh", "https"}, "The protocol to use for git operations on this host")
	c.Flags.BoolVarP(&o.web, "web", "w", false, "Authenticate with the browser-based device flow")
	c.Flags.StringArrayVarP(&o.scopes, "scopes", "s", "Additional authentication `scopes` to request")
	c.Flags.BoolVarP(&o.insecure, "insecure-storage", "", false, "Save authentication credentials in plain text (always the case in eg)")
	c.Run = func(_ *cmdutil.Command, _ []string) error { return runLogin(f, o) }
	return c
}

func runLogin(f *cmdutil.Factory, o *loginOptions) error {
	ios := f.IO
	cs := ios.ColorScheme()
	cfg, err := f.Config()
	if err != nil {
		return err
	}
	if o.web && o.withToken {
		return cmdutil.FlagErrorf("specify only one of `--web` or `--with-token`")
	}
	if len(o.scopes) > 0 && o.withToken {
		return cmdutil.FlagErrorf("specify only one of `--scopes` or `--with-token`")
	}
	if os.Getenv("EG_TOKEN") != "" {
		fmt.Fprintf(ios.ErrOut, "The value of the EG_TOKEN environment variable is being used for authentication.\n"+
			"To have eg store credentials instead, first clear the value from the environment.\n")
		return cmdutil.ErrSilent
	}
	if !o.withToken && !o.web && !ios.CanPrompt() {
		return cmdutil.FlagErrorf("--web or --with-token required when not running interactively")
	}

	prompter := &cmdutil.Prompter{IO: ios}
	hostInput := o.hostname
	if hostInput == "" {
		def, _ := cfg.DefaultHost()
		if o.withToken || !ios.CanPrompt() {
			if def == "" {
				return cmdutil.FlagErrorf("--hostname required: eg has no default ErvisioRepo server")
			}
			hostInput = def
		} else {
			if hostInput, err = prompter.Input("What is the hostname of your ErvisioRepo server?", def); err != nil {
				return err
			}
			if hostInput == "" {
				return errors.New("a hostname is required")
			}
		}
	}
	host, scheme, err := config.ParseHostInput(hostInput)
	if err != nil {
		return err
	}
	if scheme == "" {
		scheme = cfg.Scheme(host)
	}

	var token string
	setupGit := false
	if o.withToken {
		b, err := io.ReadAll(ios.In)
		if err != nil {
			return fmt.Errorf("failed to read token from standard input: %w", err)
		}
		token = strings.TrimSpace(string(b))
	} else {
		if o.gitProtocol == "" && ios.CanPrompt() {
			i, err := prompter.Select("What is your preferred protocol for Git operations on this host?", 0, []string{"HTTPS", "SSH"})
			if err != nil {
				return err
			}
			o.gitProtocol = []string{"https", "ssh"}[i]
		}
		if o.gitProtocol == "https" && ios.CanPrompt() {
			if setupGit, err = prompter.Confirm("Authenticate Git with your ErvisioRepo credentials?", true); err != nil {
				return err
			}
		}
		useWeb := o.web
		if !useWeb {
			i, err := prompter.Select("How would you like to authenticate eg?", 0,
				[]string{"Login with a web browser", "Paste an authentication token"})
			if err != nil {
				return err
			}
			useWeb = i == 0
		}
		if useWeb {
			if token, err = deviceFlow(f, scheme, host, o.scopes); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(ios.ErrOut, "\nTip: you can generate a Personal Access Token here %s\n", tokensURL(scheme, host))
			fmt.Fprintf(ios.ErrOut, "The minimum required scopes are 'repo:write' and 'user:read'.\n")
			if token, err = prompter.Password("Paste your authentication token:"); err != nil {
				return err
			}
		}
	}
	if token == "" {
		return errors.New("no token provided")
	}

	client := api.New(host, scheme, token)
	login, scopes, err := whoami(client)
	if err != nil {
		if api.IsStatus(err, http.StatusUnauthorized) {
			return fmt.Errorf("error validating token: the token was refused by %s (HTTP 401)", host)
		}
		return fmt.Errorf("error validating token: %w", err)
	}
	if err := checkScopes(scopes); err != nil {
		return fmt.Errorf("error validating token: %w", err)
	}
	if err := cfg.Login(host, login, token, o.gitProtocol, scheme); err != nil {
		return err
	}
	if o.gitProtocol != "" && !o.withToken {
		fmt.Fprintf(ios.ErrOut, "%s Configured git protocol\n", cs.SuccessIcon())
	}
	if setupGit {
		if err := configureGitHelper(f, []string{host}, cfg); err != nil {
			return err
		}
	}
	if !o.withToken || ios.IsStderrTTY() {
		fmt.Fprintf(ios.ErrOut, "%s Logged in as %s\n", cs.SuccessIcon(), cs.Bold(login))
	}
	return nil
}

func decode(resp *http.Response, v any) error {
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
