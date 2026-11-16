package auth

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
)

func newSetupGit(f *cmdutil.Factory) *cmdutil.Command {
	var hostname string
	var force bool
	c := &cmdutil.Command{
		Use:   "setup-git",
		Short: "Setup git with eg as a credential helper",
		Long: `This command configures git to use eg as a credential helper.

By default, eg will be set as the credential helper for all authenticated
hosts. If there are no authenticated hosts the command fails with an error.

Alternatively, use the --hostname flag to specify a single host to be
configured. If the host is not authenticated with, the command fails with
an error unless --force is given.`,
		Example: `# Configure git to use eg as the credential helper for all authenticated hosts
$ eg auth setup-git

# Configure git to use eg as the credential helper for a specific host
$ eg auth setup-git --hostname code.example.com`,
		Flags: cmdutil.NewFlagSet("setup-git"),
		Args:  cmdutil.NoArgs,
	}
	c.Flags.StringVarP(&hostname, "hostname", "h", "", "The `hostname` to configure git for")
	c.Flags.BoolVarP(&force, "force", "f", false, "Force setup even if the host is not known. Must be used in conjunction with --hostname")
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if force && hostname == "" {
			return cmdutil.FlagErrorf("`--force` must be used in conjunction with `--hostname`")
		}
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		hosts := cfg.Hosts()
		if hostname == "" {
			if len(hosts) == 0 {
				return errors.New("You are not logged into any ErvisioRepo hosts. Run `eg auth login` to authenticate.") //nolint:staticcheck // gh wording
			}
			return configureGitHelper(f, hosts, cfg)
		}
		h, _, err := config.ParseHostInput(hostname)
		if err != nil {
			return err
		}
		if !force && !contains(hosts, h) {
			return fmt.Errorf("You are not logged into the ErvisioRepo host %q. Run `eg auth login -h %s` to authenticate or provide `--force`", h, h) //nolint:staticcheck // gh wording
		}
		return configureGitHelper(f, []string{h}, cfg)
	}
	return c
}

// configureGitHelper points git's credential helper for each host at
// `eg auth git-credential`, as gh does.
func configureGitHelper(f *cmdutil.Factory, hosts []string, cfg *config.Config) error {
	g := f.Git("")
	helper := "!" + shellQuote(f.Executable) + " auth git-credential"
	for _, h := range hosts {
		key := "credential." + cfg.Scheme(h) + "://" + h + ".helper"
		if _, err := g.Output("config", "--global", "--replace-all", key, ""); err != nil {
			return fmt.Errorf("configure git credential helper for %s: %w", h, err)
		}
		if _, err := g.Output("config", "--global", "--add", key, helper); err != nil {
			return fmt.Errorf("configure git credential helper for %s: %w", h, err)
		}
	}
	return nil
}

func shellQuote(s string) string {
	safe := true
	for _, r := range s {
		alnum := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if !alnum && !strings.ContainsRune("/._-+:@", r) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func newGitCredential(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:    "git-credential <operation>",
		Short:  "Implements git credential helper protocol",
		Hidden: true,
		Flags:  cmdutil.NewFlagSet("git-credential"),
		Args:   cmdutil.ExactArgs(1, "the operation argument is required"),
	}
	c.Run = func(_ *cmdutil.Command, args []string) error {
		switch args[0] {
		case "store", "erase":
			return nil
		case "get":
		default:
			return fmt.Errorf("eg auth git-credential: %q operation not supported", args[0])
		}
		want := map[string]string{}
		sc := bufio.NewScanner(f.IO.In)
		for sc.Scan() {
			l := sc.Text()
			if l == "" {
				break
			}
			k, v, ok := strings.Cut(l, "=")
			if ok {
				want[k] = v
			}
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("read credential request: %w", err)
		}
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		host := strings.ToLower(want["host"])
		matched := ""
		for _, h := range cfg.Hosts() {
			if h == host && cfg.Scheme(h) == want["protocol"] {
				matched = h
				break
			}
		}
		if matched == "" {
			return cmdutil.ErrSilent
		}
		token, _ := cfg.Token(matched)
		user := cfg.User(matched)
		if token == "" {
			return cmdutil.ErrSilent
		}
		if u := want["username"]; u != "" && user != "" && u != user && u != "x-access-token" {
			if token = cfg.UserToken(matched, u); token == "" {
				return cmdutil.ErrSilent
			}
			user = u
		}
		if user == "" {
			user = "x-access-token"
		}
		out := f.IO.Out
		fmt.Fprintf(out, "protocol=%s\n", want["protocol"])
		fmt.Fprintf(out, "host=%s\n", want["host"])
		fmt.Fprintf(out, "username=%s\n", user)
		fmt.Fprintf(out, "password=%s\n", token)
		return nil
	}
	return c
}
