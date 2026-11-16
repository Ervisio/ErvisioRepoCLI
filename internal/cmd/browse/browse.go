// Package browse implements `eg browse`.
package browse

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

var shaRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// New returns the browse command.
func New(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, branch                                          string
		commit, noBrowser, settings, releases, actions, wiki, prj bool
	)
	c := &cmdutil.Command{
		Use:   "browse [<number> | <path> | <commit-sha>]",
		Short: "Open repositories, issues, pull requests, and more in the browser",
		Long: `Transition from the terminal to the web browser to view and interact with:
issues, pull requests, repository content, repository home page, and
repository settings.

The browser is $EG_BROWSER, the browser config key or $BROWSER, else the
opener of the platform. With --no-browser the URL is printed instead.`,
		Example: `$ eg browse
#=> Open the home page of the current repository

$ eg browse 217
#=> Open issue or pull request 217

$ eg browse 77507cd94ccafcf568f8560cfecde965fcfa63
#=> Open commit page

$ eg browse --settings
#=> Open repository settings

$ eg browse main.go:312
#=> Open main.go at line 312

$ eg browse main.go --branch bug-fix
#=> Open main.go with the repository at head of bug-fix branch`,
		GroupID: "core",
		Flags:   cmdutil.NewFlagSet("browse"),
		Args:    cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	fl.StringVarP(&repoFlag, "repo", "R", "", "Select another repository using the `[HOST/]OWNER/REPO` format")
	fl.StringVarP(&branch, "branch", "b", "", "Select another branch by passing in the branch name")
	fl.BoolVarP(&commit, "commit", "c", false, "Select the last commit")
	fl.BoolVarP(&noBrowser, "no-browser", "n", false, "Print destination URL instead of opening the browser")
	fl.BoolVarP(&settings, "settings", "s", false, "Open repository settings")
	fl.BoolVarP(&releases, "releases", "r", false, "Open repository releases")
	fl.BoolVarP(&actions, "actions", "a", false, "Open repository actions")
	fl.BoolVarP(&wiki, "wiki", "w", false, "Open repository wiki (not supported)")
	fl.BoolVarP(&prj, "projects", "p", false, "Open repository projects (not supported)")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if wiki {
			return cmdutil.Unsupported("Repository wikis", "")
		}
		if prj {
			return cmdutil.Unsupported("Repository projects", "")
		}
		n := 0
		for _, b := range []bool{settings, releases, actions, commit, branch != ""} {
			if b {
				n++
			}
		}
		if n > 1 || (n == 1 && len(args) > 0 && branch == "") {
			return cmdutil.FlagErrorf("arguments and flags are mutually exclusive: specify only one of `--branch`, `--commit`, `--releases`, `--settings`, `--actions`, or an argument")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		base := f.WebURL(r, "")
		var target string
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		switch {
		case settings:
			target = base + "/settings"
		case releases:
			target = base + "/releases"
		case actions:
			target = base + "/actions"
		case commit:
			sha, err := f.Git("").Output("rev-parse", "HEAD")
			if err != nil {
				return err
			}
			target = base + "/commit/" + strings.TrimSpace(sha)
		case arg == "":
			target = base
			if branch != "" {
				target += "?ref=" + url.QueryEscape(branch)
			}
		case isNumber(arg):
			target = base + "/issues/" + strings.TrimPrefix(arg, "#")
		case shaRE.MatchString(arg):
			target = base + "/commit/" + arg
		default:
			target, err = fileURL(f, r, base, repoFlag != "", arg, branch)
			if err != nil {
				return err
			}
		}
		if noBrowser {
			_, err := fmt.Fprintln(f.IO.Out, target)
			return err
		}
		return f.OpenURL(target)
	}
	return c
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	return err == nil
}

// fileURL builds the tree or blob page of a path, with an optional
// ":line" or ":start-end" suffix.
func fileURL(f *cmdutil.Factory, r ghrepo.Repo, base string, remoteOnly bool, arg, branch string) (string, error) {
	p, lines, _ := strings.Cut(arg, ":")
	if lines != "" {
		for _, part := range strings.SplitN(lines, "-", 2) {
			if _, err := strconv.Atoi(part); err != nil {
				return "", fmt.Errorf("invalid line number after colon")
			}
		}
	}
	isDir := strings.HasSuffix(p, "/")
	if !remoteOnly {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			isDir = true
		}
		if prefix, err := f.Git("").Output("rev-parse", "--show-prefix"); err == nil {
			p = path.Join(strings.TrimSpace(prefix), p)
		}
	}
	p = strings.Trim(path.Clean("/"+p), "/")
	if branch == "" {
		client, err := f.Client(r.Host)
		if err != nil {
			return "", err
		}
		var repo struct {
			DefaultBranch string `json:"default_branch"`
		}
		p := "repos/" + url.PathEscape(r.Owner) + "/" + url.PathEscape(r.Name)
		if _, err := client.REST(http.MethodGet, p, nil, &repo); err != nil {
			return "", err
		}
		branch = repo.DefaultBranch
	}
	q := url.Values{}
	if branch != "" {
		q.Set("ref", branch)
	}
	if p != "" {
		q.Set("path", p)
	}
	if isDir || p == "" {
		return base + "?" + q.Encode(), nil
	}
	u := base + "/blob?" + q.Encode()
	if lines != "" {
		start, end, _ := strings.Cut(lines, "-")
		u += "#L" + start
		if end != "" {
			u += "-L" + end
		}
	}
	return u, nil
}
