package auth

import (
	"errors"
	"fmt"
	"os"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
)

func newLogout(f *cmdutil.Factory) *cmdutil.Command {
	var hostname, user string
	c := &cmdutil.Command{
		Use:   "logout",
		Short: "Log out of an ErvisioRepo server",
		Long: `Remove authentication for an ErvisioRepo account.

This command removes the stored token from hosts.yml. It does not revoke
the token on the server: delete it under Settings → Tokens for that.`,
		Example: `# Select what host and account to log out of via a prompt
$ eg auth logout

# Log out of a specific host and account
$ eg auth logout --hostname code.example.com --user monalisa`,
		Flags: cmdutil.NewFlagSet("logout"),
		Args:  cmdutil.NoArgs,
	}
	c.Flags.StringVarP(&hostname, "hostname", "h", "", "The `hostname` of the ErvisioRepo server to log out of")
	c.Flags.StringVarP(&user, "user", "u", "", "The account to log out of")
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		ios := f.IO
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		if os.Getenv("EG_TOKEN") != "" {
			fmt.Fprintf(ios.ErrOut, "The value of the EG_TOKEN environment variable is being used for authentication.\n"+
				"To erase credentials stored in eg, first clear the value from the environment.\n")
			return cmdutil.ErrSilent
		}
		hosts := cfg.Hosts()
		if len(hosts) == 0 {
			return errors.New("not logged in to any hosts")
		}
		if hostname != "" {
			h, _, err := config.ParseHostInput(hostname)
			if err != nil {
				return err
			}
			hostname = h
		}
		type candidate struct{ host, user string }
		var cands []candidate
		for _, h := range hosts {
			if hostname != "" && h != hostname {
				continue
			}
			for _, u := range cfg.Users(h) {
				if user == "" || u == user {
					cands = append(cands, candidate{h, u})
				}
			}
		}
		switch {
		case len(cands) == 0 && hostname != "" && user == "":
			return fmt.Errorf("not logged in to %s", hostname)
		case len(cands) == 0:
			return fmt.Errorf("not logged in to %s account %s", or(hostname, "any host with"), user)
		}
		pick := cands[0]
		if len(cands) > 1 {
			if !ios.CanPrompt() {
				return cmdutil.FlagErrorf("unable to determine which account to log out of, please specify `--hostname` and `--user`")
			}
			opts := make([]string, len(cands))
			for i, c := range cands {
				opts[i] = c.user + " (" + c.host + ")"
			}
			i, err := (&cmdutil.Prompter{IO: ios}).Select("What account do you want to log out of?", 0, opts)
			if err != nil {
				return err
			}
			pick = cands[i]
		}
		if err := cfg.Logout(pick.host, pick.user); err != nil {
			return err
		}
		cs := ios.ColorScheme()
		fmt.Fprintf(ios.ErrOut, "%s Logged out of %s account %s\n", cs.SuccessIcon(), pick.host, cs.Bold(pick.user))
		return nil
	}
	return c
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
