package search

import (
	"fmt"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

type codeItem struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	SHA        string   `json:"sha"`
	HTMLURL    string   `json:"html_url"`
	Repository repoItem `json:"repository"`
	TextMatch  []struct {
		Fragment string `json:"fragment"`
		Property string `json:"property"`
	} `json:"text_matches"`
}

func (c *codeItem) export() map[string]any {
	matches := make([]any, 0, len(c.TextMatch))
	for _, m := range c.TextMatch {
		matches = append(matches, map[string]any{"fragment": m.Fragment, "property": m.Property})
	}
	return map[string]any{"path": c.Path, "repository": c.Repository.export(), "sha": c.SHA,
		"textMatches": matches, "url": c.HTMLURL}
}

var codeJSONFields = []string{"path", "repository", "sha", "textMatches", "url"}

func newCode(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlags, owners                   []string
		filename, extension, language, size string
		match                               []string
		limit                               int
		exporter                            cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "code <query>",
		Short: "Search within code",
		Long: `Search within code in ErvisioRepo repositories.

The server searches the default branch of one repository (case-insensitive
text, one result per file): the repository comes from --repo, a repo:
qualifier, or the git remotes of the current directory. path: limits the
directory; --language, --extension and --filename are not supported by the
server and are ignored there.`,
		Example: `# search code matching "react" and "lifecycle"
$ eg search code react lifecycle --repo acme/widgets

# search code matching "error handling" in a directory
$ eg search code "error handling" path:internal`,
		Flags: cmdutil.NewFlagSet("code"),
		Args:  cmdutil.RangeArgs(1, 1<<20, "specify search keywords"),
	}
	fl := c.Flags
	fl.StringArrayVarP(&repoFlags, "repo", "R", "Filter on `repository`")
	fl.StringArrayVarP(&owners, "owner", "", "Filter on `owner`")
	fl.StringArrayVarP(&match, "match", "", "Restrict search to file contents or file path: {file|path}")
	fl.StringVarP(&filename, "filename", "", "", "Filter on filename")
	fl.StringVarP(&extension, "extension", "", "", "Filter on file extension")
	fl.StringVarP(&language, "language", "", "", "Filter results by language")
	fl.StringVarP(&size, "size", "", "", "Filter on size range, in kilobytes")
	fl.IntVarP(&limit, "limit", "L", 30, "Maximum number of code results to fetch")
	cmdutil.AddJSONFlags(c, &exporter, codeJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		if limit < 1 || limit > 1000 {
			return cmdutil.FlagErrorf("`--limit` must be between 1 and 1000")
		}
		q := &query{keywords: args}
		for _, o := range owners {
			q.add("user", o)
		}
		for _, m := range match {
			q.add("in", m)
		}
		q.add("filename", filename)
		q.add("extension", extension)
		q.add("language", language)
		q.add("size", size)
		repos, err := parseRepos(f, repoFlags)
		if err != nil {
			return err
		}
		if repos, err = needRepo(f, q, repos, "code"); err != nil {
			return err
		}
		host, err := searchHost(f, repos)
		if err != nil {
			return err
		}
		client, err := f.Client(host)
		if err != nil {
			return err
		}
		items, total, err := perRepo[codeItem](client, "code", q, repos, limit)
		if err != nil {
			return err
		}
		if exporter.Enabled() {
			out := make([]map[string]any, len(items))
			for i := range items {
				out[i] = exporter.Pick(items[i].export())
			}
			return exporter.Write(f.IO, out)
		}
		if len(items) == 0 {
			return noResults(f, "code")
		}
		header(f, len(items), total, "results")
		cs := f.IO.ColorScheme()
		out := f.IO.Out
		for _, it := range items {
			if !f.IO.IsStdoutTTY() {
				if len(it.TextMatch) == 0 {
					fmt.Fprintf(out, "%s:%s\n", it.Repository.FullName, it.Path)
				}
				for _, m := range it.TextMatch {
					fmt.Fprintf(out, "%s:%s: %s\n", it.Repository.FullName, it.Path, m.Fragment)
				}
				continue
			}
			fmt.Fprintf(out, "%s %s\n", cs.Blue(it.Repository.FullName), cs.Green(it.Path))
			for _, m := range it.TextMatch {
				fmt.Fprintf(out, "    %s\n", m.Fragment)
			}
		}
		return nil
	}
	return c
}
