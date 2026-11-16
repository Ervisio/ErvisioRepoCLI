// Package root assembles the command tree and maps errors to exit codes.
package root

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/build"
	apicmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/api"
	authcmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/auth"
	browsecmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/browse"
	issuecmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/issue"
	labelcmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/label"
	prcmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/pr"
	releasecmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/release"
	repocmd "github.com/Ervisio/ErvisioRepoCLI/internal/cmd/repo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// Exit codes, as gh.
const (
	exitOK     = 0
	exitError  = 1
	exitCancel = 2
	exitAuth   = 4
	exitPend   = 8
)

// New builds the command tree.
func New(f *cmdutil.Factory) *cmdutil.Command {
	root := &cmdutil.Command{
		Use:   "eg <command> <subcommand> [flags]",
		Short: "Ervisio CLI",
		Long:  "Work seamlessly with ErvisioRepo from the command line.",
		Example: `$ eg repo clone acme/widgets
$ eg repo list
$ eg api repos/{owner}/{repo}/releases`,
		IO: f.IO,
	}
	root.AddGroup(cmdutil.Group{ID: "core", Title: "CORE COMMANDS"})
	root.AddCommand(authcmd.New(f), browsecmd.New(f), issuecmd.New(f), prcmd.New(f), releasecmd.New(f), repocmd.New(f),
		labelcmd.New(f), apicmd.New(f), newVersion(f), newHelp(f, root))
	return root
}

func versionText() string {
	v := build.Version
	date := ""
	if build.Date != "" {
		date = " (" + build.Date + ")"
	}
	url := "https://github.com/Ervisio/ErvisioRepoCLI/releases/latest"
	if releaseRE.MatchString(v) {
		url = "https://github.com/Ervisio/ErvisioRepoCLI/releases/tag/v" + strings.TrimPrefix(v, "v")
	}
	return fmt.Sprintf("eg version %s%s\n%s\n", v, date, url)
}

var releaseRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func newVersion(f *cmdutil.Factory) *cmdutil.Command {
	return &cmdutil.Command{
		Use:    "version",
		Short:  "Show eg version",
		Hidden: true,
		Args:   cmdutil.NoArgs,
		Run: func(*cmdutil.Command, []string) error {
			fmt.Fprint(f.IO.Out, versionText())
			return nil
		},
	}
}

func newHelp(f *cmdutil.Factory, root *cmdutil.Command) *cmdutil.Command {
	return &cmdutil.Command{
		Use:    "help [command | topic]",
		Short:  "Help about any command",
		Hidden: true,
		Run: func(_ *cmdutil.Command, args []string) error {
			if len(args) == 1 {
				if t, ok := topics[args[0]]; ok {
					fmt.Fprint(f.IO.Out, t)
					return nil
				}
			}
			c := root
			for _, a := range args {
				sub := c.Sub(a)
				if sub == nil {
					return fmt.Errorf("unknown help topic %q", strings.Join(args, " "))
				}
				c = sub
			}
			c.PrintHelp()
			return nil
		},
	}
}

var topics = map[string]string{
	"environment": `EG_HOST: specify the ErvisioRepo hostname for commands that would otherwise
assume the first logged-in host when not in a context of an existing
repository. It may carry a scheme, e.g. http://localhost:8080.

EG_TOKEN: an authentication token for API requests. Setting this avoids
being prompted to authenticate and takes precedence over previously stored
credentials. It applies to every host.

EG_REPO: specify the repository in the "[HOST/]OWNER/REPO" format for
commands that otherwise operate on a local repository.

EG_CONFIG_DIR: the directory where eg will store configuration files.
If not specified, the default value will be one of the following paths
(in order of precedence): "$XDG_CONFIG_HOME/eg" (if $XDG_CONFIG_HOME is
set), "$AppData/eg" (Windows only), "$HOME/.config/eg".

EG_BROWSER, BROWSER: the web browser to use for opening links, as a
command line split into words like a shell would (no shell is involved).
The browser key of config.yml sits between the two. By default eg uses
open on macOS, xdg-open on Linux and rundll32 on Windows.

EG_EDITOR, VISUAL, EDITOR (in this order, with the editor key of
config.yml after EG_EDITOR): the editor for authoring text, e.g. issue
and pull request bodies. Defaults to nano (notepad on Windows).

EG_PAGER, PAGER (with the pager key of config.yml after EG_PAGER): a
terminal paging program to send standard output to, e.g. "less". Defaults
to "less -R"; set EG_PAGER to "" or "cat" to disable paging. LESS=FRX and
LV=-c are set when unset.

EG_DEBUG: set to a truthy value to enable verbose output of HTTP requests
on standard error.

EG_FORCE_TTY: set to any value to force terminal-style output even when
the output is redirected.

EG_PROMPT_DISABLED: set to any value to disable interactive prompting in
the terminal.

NO_COLOR: set to any value to avoid printing ANSI escape sequences for
color output.

CLICOLOR_FORCE: set to a value other than "0" to keep ANSI colors in the
output even when the output is piped.
`,
	"exit-codes": `eg follows normal conventions regarding exit codes.

- If a command completes successfully, the exit code will be 0

- If a command fails for any reason, the exit code will be 1

- If a command is running but gets cancelled, the exit code will be 2

- If a command requires authentication, the exit code will be 4

NOTE: It is possible that a particular command may have more exit codes,
so it is a good practice to check documentation for the command if you
are relying on exit codes to control some behavior.
`,
}

// Main runs eg with the arguments after the program name and returns the
// exit code.
func Main(args []string) int {
	f := cmdutil.NewFactory()
	root := New(f)
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
		fmt.Fprint(f.IO.Out, versionText())
		return exitOK
	}
	err := root.Execute(args)
	if err == nil {
		return exitOK
	}
	stderr := f.IO.ErrOut
	var (
		flagErr *cmdutil.FlagError
		authErr *cmdutil.AuthError
		httpErr *api.HTTPError
	)
	switch {
	case errors.Is(err, cmdutil.ErrSilent):
		return exitError
	case errors.Is(err, cmdutil.ErrCancel):
		return exitCancel
	case errors.Is(err, cmdutil.ErrPending):
		return exitPend
	case errors.As(err, &authErr):
		fmt.Fprintln(stderr, err)
		return exitAuth
	case errors.As(err, &flagErr):
		fmt.Fprintln(stderr, err)
		cmd := flagErr.Cmd
		if cmd == nil {
			cmd = root
		}
		fmt.Fprintf(stderr, "\n%s", cmd.UsageString())
		return exitError
	}
	fmt.Fprintln(stderr, err)
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
		host := ""
		if httpErr.RequestURL != nil {
			host = httpErr.RequestURL.Host
		}
		fmt.Fprintf(stderr, "Try authenticating with:  eg auth login --hostname %s\n", host)
	}
	return exitError
}
