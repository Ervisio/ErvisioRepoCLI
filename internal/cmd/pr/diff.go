package pr

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

func newDiff(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, color      string
		patch, nameOnly, web bool
	)
	c := &cmdutil.Command{
		Use:   "diff " + argUse,
		Short: "View changes in a pull request",
		Long: `View changes in a pull request.

Without an argument, the pull request that belongs to the current branch
is selected.

With '--web', open the pull request diff in a web browser instead.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("diff"),
		Args:    selectorArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.EnumVarP(&color, "color", "", "auto", []string{"always", "never", "auto"}, "Use color in diff output")
	fl.BoolVarP(&patch, "patch", "", false, "Display diff in patch format")
	fl.BoolVarP(&nameOnly, "name-only", "", false, "Display only names of changed files")
	fl.BoolVarP(&web, "web", "w", false, "Open the pull request diff in the browser")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		if web {
			return f.OpenURL(p.HTMLURL + "?tab=files")
		}
		if nameOnly {
			next := fmt.Sprintf("%s/pulls/%d/files?per_page=100", shared.RepoPath(fd.repo), p.Number)
			for next != "" {
				var files []struct {
					Filename string `json:"filename"`
				}
				if next, err = fd.client.REST(http.MethodGet, next, nil, &files); err != nil {
					return err
				}
				for _, fi := range files {
					fmt.Fprintln(f.IO.Out, fi.Filename)
				}
			}
			return nil
		}
		accept := "application/vnd.github.diff"
		if patch {
			accept = "application/vnd.github.patch"
		}
		u := fd.client.RESTURL(fmt.Sprintf("%s/pulls/%d", shared.RepoPath(fd.repo), p.Number))
		resp, err := fd.client.Request(http.MethodGet, u, nil, http.Header{"Accept": {accept}})
		if err != nil {
			return err
		}
		defer resp.Body.Close() //nolint:errcheck // read-only body
		if resp.StatusCode == http.StatusNotAcceptable {
			return fmt.Errorf("could not find pull request diff: the diff is too large or the series has too many commits (HTTP 406)")
		}
		if resp.StatusCode >= 300 {
			return api.HandleHTTPError(resp)
		}
		useColor := color == "always" || color == "auto" && f.IO.ColorEnabled()
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(f.IO.ErrOut, "failed to start pager: %v\n", err)
		}
		defer f.StopPager()
		if !useColor {
			_, err = io.Copy(f.IO.Out, resp.Body)
			return err
		}
		return colorDiff(f.IO.Out, resp.Body)
	}
	return c
}

// colorDiff colors a unified diff as gh does.
func colorDiff(w io.Writer, r io.Reader) error {
	const (
		header   = "\x1b[1;38m"
		addition = "\x1b[32m"
		removal  = "\x1b[31m"
		hunk     = "\x1b[36m"
		reset    = "\x1b[m"
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		l := sc.Text()
		color := ""
		switch {
		case strings.HasPrefix(l, "diff --git"), strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"),
			strings.HasPrefix(l, "index "), strings.HasPrefix(l, "new file"), strings.HasPrefix(l, "deleted file"):
			color = header
		case strings.HasPrefix(l, "@@"):
			color = hunk
		case strings.HasPrefix(l, "+"):
			color = addition
		case strings.HasPrefix(l, "-"):
			color = removal
		}
		var err error
		if color != "" {
			_, err = fmt.Fprintf(w, "%s%s%s\n", color, l, reset)
		} else {
			_, err = fmt.Fprintln(w, l)
		}
		if err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read diff: %w", err)
	}
	return nil
}
