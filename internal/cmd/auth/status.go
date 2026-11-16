package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
)

func newStatus(f *cmdutil.Factory) *cmdutil.Command {
	var hostname string
	var showToken, active bool
	c := &cmdutil.Command{
		Use:   "status",
		Short: "Display active account and authentication state on each known ErvisioRepo host",
		Long: `Display active account and authentication state on each known ErvisioRepo host.

For each host, the authentication state of each known account is tested and
any issues are included in the output.`,
		Flags: cmdutil.NewFlagSet("status"),
		Args:  cmdutil.NoArgs,
	}
	c.Flags.StringVarP(&hostname, "hostname", "h", "", "Check only a specific `hostname`'s auth status")
	c.Flags.BoolVarP(&showToken, "show-token", "t", false, "Display the auth token")
	c.Flags.BoolVarP(&active, "active", "a", false, "Display the active account only")
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		ios := f.IO
		cs := ios.ColorScheme()
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		hosts := cfg.Hosts()
		if len(hosts) == 0 {
			fmt.Fprintf(ios.ErrOut, "You are not logged into any ErvisioRepo hosts. To log in, run: %s\n", cs.Bold("eg auth login"))
			return cmdutil.ErrSilent
		}
		if hostname != "" {
			h, _, err := config.ParseHostInput(hostname)
			if err != nil {
				return err
			}
			if !contains(hosts, h) {
				fmt.Fprintf(ios.ErrOut, "You are not logged into any accounts on %s\n", h)
				return cmdutil.ErrSilent
			}
			hosts = []string{h}
		}
		failed := false
		var b strings.Builder
		for i, h := range hosts {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(cs.Bold(h) + "\n")
			token, source := cfg.Token(h)
			user := cfg.User(h)
			login, scopes, err := whoami(api.New(h, cfg.Scheme(h), token))
			if user == "" {
				user = login
			}
			label := source
			switch {
			case err != nil && api.IsStatus(err, http.StatusUnauthorized):
				failed = true
				fmt.Fprintf(&b, "  %s Failed to log in to %s account %s (%s)\n", cs.FailureIcon(), h, cs.Bold(or(user, "unknown")), label)
				fmt.Fprintf(&b, "  - Active account: true\n")
				fmt.Fprintf(&b, "  - The token in %s is invalid.\n", label)
				fmt.Fprintf(&b, "  - To re-authenticate, run: %s\n", cs.Bold("eg auth login -h "+h))
				fmt.Fprintf(&b, "  - To forget about this account, run: %s\n", cs.Bold("eg auth logout -h "+h+" -u "+user))
				continue
			case err != nil:
				failed = true
				fmt.Fprintf(&b, "  %s Failed to log in to %s using token (%s)\n", cs.FailureIcon(), h, label)
				fmt.Fprintf(&b, "  - Active account: true\n")
				fmt.Fprintf(&b, "  - %s\n", err)
				continue
			}
			fmt.Fprintf(&b, "  %s Logged in to %s account %s (%s)\n", cs.SuccessIcon(), h, cs.Bold(login), label)
			fmt.Fprintf(&b, "  - Active account: true\n")
			fmt.Fprintf(&b, "  - Git operations protocol: %s\n", cs.Bold(cfg.GitProtocol(h)))
			fmt.Fprintf(&b, "  - Token: %s\n", cs.Bold(displayToken(token, showToken)))
			fmt.Fprintf(&b, "  - Token scopes: %s\n", scopeList(scopes))
			if err := checkScopes(scopes); err != nil {
				fmt.Fprintf(&b, "  %s %s\n", cs.WarningIcon(), err)
			}
			if active {
				continue
			}
			for _, u := range cfg.Users(h) {
				if u == login || u == user {
					continue
				}
				fmt.Fprintf(&b, "\n  %s Logged in to %s account %s (%s)\n", cs.SuccessIcon(), h, cs.Bold(u), cfg.HostsPath())
				fmt.Fprintf(&b, "  - Active account: false\n")
				fmt.Fprintf(&b, "  - Git operations protocol: %s\n", cs.Bold(cfg.GitProtocol(h)))
			}
		}
		fmt.Fprint(ios.Out, b.String())
		if failed {
			return cmdutil.ErrSilent
		}
		return nil
	}
	return c
}

func displayToken(t string, show bool) string {
	if show {
		return t
	}
	prefix := ""
	if i := strings.IndexByte(t, '_'); i > 0 && i < 6 {
		prefix = t[:i+1]
	}
	return prefix + strings.Repeat("*", len(t)-len(prefix))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
