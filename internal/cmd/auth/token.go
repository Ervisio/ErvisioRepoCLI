package auth

import (
	"fmt"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
)

func newToken(f *cmdutil.Factory) *cmdutil.Command {
	var hostname, user string
	c := &cmdutil.Command{
		Use:   "token",
		Short: "Print the authentication token eg uses for a hostname and account",
		Long: `Print the authentication token eg uses for a hostname and account.

Without --hostname, the default host is used ($EG_HOST, else the first
logged-in host).`,
		Flags: cmdutil.NewFlagSet("token"),
		Args:  cmdutil.NoArgs,
	}
	c.Flags.StringVarP(&hostname, "hostname", "h", "", "The `hostname` of the ErvisioRepo server")
	c.Flags.StringVarP(&user, "user", "u", "", "The account to output the token for")
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		host := hostname
		if host == "" {
			host, _ = cfg.DefaultHost()
		} else if host, _, err = config.ParseHostInput(host); err != nil {
			return err
		}
		if host == "" {
			return fmt.Errorf("no oauth token found: not logged in to any host")
		}
		var token string
		if user != "" {
			token = cfg.UserToken(host, user)
			if token == "" {
				return fmt.Errorf("no oauth token found for %s account %s", host, user)
			}
		} else if token, _ = cfg.Token(host); token == "" {
			return fmt.Errorf("no oauth token found for %s", host)
		}
		fmt.Fprintln(f.IO.Out, token)
		return nil
	}
	return c
}
