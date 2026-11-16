package issue

import (
	"fmt"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

var issueStatusFields = []string{"assignees", "author", "body", "closed", "closedAt", "createdAt", "id",
	"labels", "number", "state", "stateReason", "title", "updatedAt", "url"}

func newStatus(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag string
		exporter cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "status",
		Short:   "Show status of relevant issues",
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("status"),
		Args:    cmdutil.NoArgs,
	}
	shared.AddRepoFlag(c, &repoFlag)
	cmdutil.AddJSONFlags(c, &exporter, issueStatusFields)
	c.Run = func(_ *cmdutil.Command, _ []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		base := "repo:" + r.FullName() + " is:issue is:open "
		sections := []struct{ key, title, empty, q string }{
			{"assigned", "Issues assigned to you", "There are no issues assigned to you", "assignee:@me"},
			{"mentioned", "Issues mentioning you", "There are no issues mentioning you", "mentions:@me"},
			{"authored", "Issues opened by you", "There are no issues opened by you", "author:@me"},
		}
		results := make([][]*shared.Issue, len(sections))
		for i, s := range sections {
			items, _, err := shared.Search(client, base+s.q, 10)
			if err != nil {
				return err
			}
			for j := range items {
				results[i] = append(results[i], items[j].SearchToIssue())
			}
		}
		if exporter.Enabled() {
			out := map[string]any{}
			for i, s := range sections {
				list := []any{}
				for _, is := range results[i] {
					list = append(list, exporter.Pick(is.ExportFields()))
				}
				out[s.key] = list
			}
			return exporter.Write(f.IO, out)
		}
		ios := f.IO
		cs := ios.ColorScheme()
		out := ios.Out
		fmt.Fprintf(out, "\nRelevant issues in %s\n\n", r.FullName())
		for i, s := range sections {
			fmt.Fprintln(out, cs.Bold(s.title))
			if len(results[i]) == 0 {
				fmt.Fprintf(out, "  %s\n\n", cs.Gray(s.empty))
				continue
			}
			for _, is := range results[i] {
				labels := ""
				if ls := shared.LabelNames(is.Labels); len(ls) > 0 {
					labels = " " + cs.Gray("("+strings.Join(ls, ", ")+")")
				}
				fmt.Fprintf(out, "  %s  %s%s  %s\n", cs.Green(fmt.Sprintf("#%d", is.Number)), is.Title, labels, cs.Gray(shared.Ago(is.UpdatedAt)))
			}
			fmt.Fprintln(out)
		}
		return nil
	}
	return c
}
