package repo

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

func gitignoreNames(c *api.Client) ([]string, error) {
	var names []string
	if _, err := c.REST(http.MethodGet, "gitignore/templates", nil, &names); err != nil {
		if api.IsNotFound(err) {
			return nil, cmdutil.Unsupported("Gitignore templates", "GET /gitignore/templates")
		}
		return nil, err
	}
	return names, nil
}

type license struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	SPDXID  string `json:"spdx_id"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

func listLicenses(c *api.Client) ([]license, error) {
	var out []license
	if _, err := c.REST(http.MethodGet, "licenses", nil, &out); err != nil {
		if api.IsNotFound(err) {
			return nil, cmdutil.Unsupported("License templates", "GET /licenses")
		}
		return nil, err
	}
	return out, nil
}

// hostClient returns a client for the default host.
func hostClient(f *cmdutil.Factory) (*api.Client, error) {
	host, err := f.DefaultHost()
	if err != nil {
		return nil, err
	}
	return f.Client(host)
}

func newGitignore(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "gitignore <command>",
		Short:   "List and view available repository gitignore templates",
		GroupID: "general",
	}
	c.AddCommand(&cmdutil.Command{
		Use:     "list",
		Short:   "List available repository gitignore templates",
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgs,
		Run: func(*cmdutil.Command, []string) error {
			client, err := hostClient(f)
			if err != nil {
				return err
			}
			names, err := gitignoreNames(client)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				return fmt.Errorf("no gitignore templates found")
			}
			if f.IO.IsStdoutTTY() {
				fmt.Fprintln(f.IO.Out, f.IO.ColorScheme().Bold("GITIGNORE"))
			}
			for _, n := range names {
				fmt.Fprintln(f.IO.Out, n)
			}
			return nil
		},
	}, &cmdutil.Command{
		Use:   "view <template>",
		Short: "View an available repository gitignore template",
		Long: `View an available repository .gitignore template.

<template> is a case-sensitive .gitignore template name.

For a list of available templates, run eg repo gitignore list.`,
		Example: `# View the Go gitignore template
$ eg repo gitignore view Go

# Create a new .gitignore file using the Go template
$ eg repo gitignore view Go > .gitignore`,
		Args: cmdutil.ExactArgs(1, "cannot view gitignore template: template argument required"),
		Run: func(_ *cmdutil.Command, args []string) error {
			client, err := hostClient(f)
			if err != nil {
				return err
			}
			var t struct {
				Source string `json:"source"`
			}
			if _, err := client.REST(http.MethodGet, "gitignore/templates/"+url.PathEscape(args[0]), nil, &t); err != nil {
				if api.IsNotFound(err) {
					return fmt.Errorf("'%s' is not a valid gitignore template. Run `eg repo gitignore list` for options", args[0])
				}
				return err
			}
			fmt.Fprint(f.IO.Out, t.Source)
			return nil
		},
	})
	return c
}

func newLicense(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "license <command>",
		Short:   "Explore repository licenses",
		GroupID: "general",
	}
	var web bool
	view := &cmdutil.Command{
		Use:   "view {<license-key> | <spdx-id>}",
		Short: "View a specific repository license",
		Long: `View a specific repository license by license key or SPDX ID.

Run eg repo license list to see available commonly used licenses.`,
		Example: `# View the MIT license
$ eg repo license view MIT

# Create a LICENSE.md with the MIT license
$ eg repo license view MIT > LICENSE.md`,
		Flags: cmdutil.NewFlagSet("view"),
		Args:  cmdutil.ExactArgs(1, "cannot view license: license argument required"),
	}
	view.Flags.BoolVarP(&web, "web", "w", false, "Open https://choosealicense.com/ in the browser")
	view.Run = func(_ *cmdutil.Command, args []string) error {
		client, err := hostClient(f)
		if err != nil {
			return err
		}
		var l license
		if _, err := client.REST(http.MethodGet, "licenses/"+url.PathEscape(args[0]), nil, &l); err != nil {
			if api.IsNotFound(err) {
				return fmt.Errorf("'%s' is not a valid license name or SPDX ID.\n\nRun `eg repo license list` to see available commonly used licenses", args[0])
			}
			return err
		}
		if web {
			u := l.HTMLURL
			if u == "" {
				u = "https://choosealicense.com/licenses/" + l.Key
			}
			return f.OpenURL(u)
		}
		fmt.Fprint(f.IO.Out, l.Body)
		return nil
	}
	c.AddCommand(&cmdutil.Command{
		Use:     "list",
		Short:   "List common repository licenses",
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgs,
		Run: func(*cmdutil.Command, []string) error {
			client, err := hostClient(f)
			if err != nil {
				return err
			}
			lics, err := listLicenses(client)
			if err != nil {
				return err
			}
			if len(lics) == 0 {
				return fmt.Errorf("no repository licenses found")
			}
			t := cmdutil.NewTable(f.IO)
			t.Header("License key", "SPDX ID", "License name")
			for _, l := range lics {
				t.Field(l.Key, nil, false)
				t.Field(l.SPDXID, nil, false)
				t.Field(l.Name, nil, true)
				t.EndRow()
			}
			return t.Render()
		},
	}, view)
	return c
}
