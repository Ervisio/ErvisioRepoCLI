package repo

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

func newView(f *cmdutil.Factory) *cmdutil.Command {
	var (
		web      bool
		branch   string
		exporter cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "view [<repository>]",
		Short: "View a repository",
		Long: `Display the description and the README of an ErvisioRepo repository.

With no argument, the repository for the current directory is displayed.

With '--web', the repository URL is printed so you can open it in a
browser (eg does not launch one).

With '--branch', view a specific branch of the repository.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("view"),
		Args:    cmdutil.MaxArgs(1),
	}
	c.Flags.BoolVarP(&web, "web", "w", false, "Open a repository in the browser")
	c.Flags.StringVarP(&branch, "branch", "b", "", "View a specific branch of the repository")
	cmdutil.AddJSONFlags(c, &exporter, repoJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		var r ghrepo.Repo
		var err error
		if len(args) > 0 {
			r, err = resolveArg(f, args[0])
		} else {
			r, err = f.BaseRepo("")
		}
		if err != nil {
			return err
		}
		if web {
			u := f.WebURL(r, "")
			if branch != "" {
				u += "?ref=" + url.QueryEscape(branch)
			}
			return f.OpenURL(u)
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		repo, err := fetchRepo(client, r)
		if err != nil {
			return err
		}
		if exporter.Enabled() {
			return exporter.Write(f.IO, exporter.Pick(repo.exportFields()))
		}
		readme, err := fetchReadme(client, r, branch)
		if err != nil {
			return err
		}
		ios := f.IO
		cs := ios.ColorScheme()
		out := ios.Out
		if !ios.IsStdoutTTY() {
			fmt.Fprintf(out, "name:\t%s\n", repo.FullName)
			fmt.Fprintf(out, "description:\t%s\n", repo.description())
			if readme != "" {
				fmt.Fprintln(out, "--")
				fmt.Fprint(out, readme)
				if !strings.HasSuffix(readme, "\n") {
					fmt.Fprintln(out)
				}
			}
			return nil
		}
		fmt.Fprintln(out, cs.Bold(repo.FullName))
		if d := repo.description(); d != "" {
			fmt.Fprintln(out, d)
		} else {
			fmt.Fprintln(out, cs.Gray("No description provided"))
		}
		fmt.Fprintln(out)
		if readme == "" {
			fmt.Fprintln(out, cs.Gray("This repository does not have a README"))
		} else {
			fmt.Fprintln(out, renderMarkdown(readme, cs.Bold))
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Gray("View this repository on ErvisioRepo: "+repo.HTMLURL))
		return nil
	}
	return c
}

// fetchReadme returns the decoded README, "" when there is none.
func fetchReadme(c *api.Client, r ghrepo.Repo, ref string) (string, error) {
	p := repoPath(r) + "/readme"
	if ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	var res struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if _, err := c.REST(http.MethodGet, p, nil, &res); err != nil {
		if api.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	if res.Encoding != "base64" {
		return res.Content, nil
	}
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(res.Content, "\n", ""))
	if err != nil {
		return "", fmt.Errorf("decode README: %w", err)
	}
	return string(b), nil
}

// renderMarkdown prints Markdown plainly: headings in bold, everything
// else as written.
func renderMarkdown(md string, bold func(string) string) string {
	lines := strings.Split(strings.TrimRight(md, "\n"), "\n")
	inFence := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			lines[i] = "  " + l
			continue
		}
		if !inFence && strings.HasPrefix(t, "#") {
			lines[i] = "  " + bold(strings.TrimSpace(strings.TrimLeft(t, "#")))
			continue
		}
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}
