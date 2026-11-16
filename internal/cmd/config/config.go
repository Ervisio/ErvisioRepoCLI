// Package config implements `eg config`.
package config

import (
	"fmt"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// option is a known configuration key.
type option struct {
	key, desc, def string
	values         []string
}

var options = []option{
	{key: "git_protocol", desc: "the protocol to use for git clone and push operations", def: "https", values: []string{"https", "ssh"}},
	{key: "editor", desc: "the text editor program to use for authoring text"},
	{key: "prompt", desc: "toggle interactive prompting in the terminal", def: "enabled", values: []string{"enabled", "disabled"}},
	{key: "pager", desc: "the terminal pager program to send standard output to"},
	{key: "browser", desc: "the web browser to use for opening URLs"},
}

func find(key string) *option {
	for i := range options {
		if options[i].key == key {
			return &options[i]
		}
	}
	return nil
}

func help() string {
	var b strings.Builder
	for _, o := range options {
		fmt.Fprintf(&b, "- %s: %s", o.key, o.desc)
		if len(o.values) > 0 {
			fmt.Fprintf(&b, " {%s}", strings.Join(o.values, "|"))
		}
		if o.def != "" {
			fmt.Fprintf(&b, " (default %s)", o.def)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// New returns the config command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:   "config <command>",
		Short: "Manage configuration for eg",
		Long: `Display or change configuration settings for eg.

Current respected settings:
` + help(),
	}
	c.AddCommand(newGet(f), newSet(f), newList(f))
	return c
}

func newGet(f *cmdutil.Factory) *cmdutil.Command {
	var host string
	c := &cmdutil.Command{
		Use:   "get <key>",
		Short: "Print the value of a given configuration key",
		Example: `$ eg config get git_protocol
https`,
		Flags: cmdutil.NewFlagSet("get"),
		Args:  cmdutil.ExactArgs(1, "cannot get value: key argument required"),
	}
	c.Flags.StringVarP(&host, "host", "h", "", "Get per-host setting")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		v := cfg.Get(host, args[0])
		if v == "" {
			if o := find(args[0]); o != nil {
				v = o.def
			}
		}
		if v == "" {
			return cmdutil.ErrSilent
		}
		fmt.Fprintln(f.IO.Out, v)
		return nil
	}
	return c
}

func newSet(f *cmdutil.Factory) *cmdutil.Command {
	var host string
	c := &cmdutil.Command{
		Use:   "set <key> <value>",
		Short: "Update configuration with a value for the given key",
		Example: `$ eg config set editor vim
$ eg config set editor "code --wait"
$ eg config set git_protocol ssh --host code.example.com
$ eg config set prompt disabled`,
		Flags: cmdutil.NewFlagSet("set"),
		Args:  cmdutil.ExactArgs(2, "cannot set value: key and value arguments required"),
	}
	c.Flags.StringVarP(&host, "host", "h", "", "Set per-host setting")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		key, value := args[0], args[1]
		o := find(key)
		if o == nil {
			fmt.Fprintf(f.IO.ErrOut, "%s warning: '%s' is not a known configuration key\n", f.IO.ColorScheme().WarningIcon(), key)
		} else if len(o.values) > 0 {
			ok := false
			for _, v := range o.values {
				ok = ok || v == value
			}
			if !ok {
				return fmt.Errorf("failed to set %q to %q: valid values are '%s'", key, value, strings.Join(o.values, "', '"))
			}
		}
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		if host != "" {
			if h := cmdutil.MatchHost(cfg.Hosts(), host); h != "" {
				host = h
			}
		}
		if err := cfg.Set(host, key, value); err != nil {
			return fmt.Errorf("failed to write config to disk: %w", err)
		}
		return nil
	}
	return c
}

func newList(f *cmdutil.Factory) *cmdutil.Command {
	var host string
	c := &cmdutil.Command{
		Use:     "list",
		Short:   "Print a list of configuration keys and values",
		Aliases: []string{"ls"},
		Flags:   cmdutil.NewFlagSet("list"),
		Args:    cmdutil.NoArgs,
	}
	c.Flags.StringVarP(&host, "host", "h", "", "Get per-host configuration")
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		for _, o := range options {
			v := cfg.Get(host, o.key)
			if v == "" {
				v = o.def
			}
			fmt.Fprintf(f.IO.Out, "%s=%s\n", o.key, v)
		}
		return nil
	}
	return c
}
