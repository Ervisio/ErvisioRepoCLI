package pr

import (
	"fmt"
	"io"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

func newStatus(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag string
		exporter cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:     "status",
		Short:   "Show status of relevant pull requests",
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("status"),
		Args:    cmdutil.NoArgs,
	}
	shared.AddRepoFlag(c, &repoFlag)
	cmdutil.AddJSONFlags(c, &exporter, shared.PullJSONFields)
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
		var current *shared.PullRequest
		branch := ""
		if repoFlag == "" {
			if b, err := f.Git("").CurrentBranch(); err == nil {
				branch = b
				if n := checkedOutPull(f); n > 0 {
					current, _ = shared.FetchPull(client, r, n)
				} else if current, err = pullForBranch(client, r, headOwner(f, r, b), b); err != nil {
					return err
				}
			}
		}
		base := "repo:" + r.FullName() + " is:pr is:open "
		var created, review []*shared.PullRequest
		for _, s := range []struct {
			q   string
			dst *[]*shared.PullRequest
		}{{"author:@me", &created}, {"review-requested:@me", &review}} {
			items, _, err := shared.Search(client, base+s.q, 10)
			if err != nil {
				return err
			}
			for i := range items {
				*s.dst = append(*s.dst, items[i].SearchToPull())
			}
		}
		if exporter.Enabled() {
			pick := func(list []*shared.PullRequest) []any {
				out := []any{}
				for _, p := range list {
					out = append(out, exporter.Pick(p.ExportFields()))
				}
				return out
			}
			var cur any
			if current != nil {
				cur = exporter.Pick(current.ExportFields())
			}
			return exporter.Write(f.IO, map[string]any{"currentBranch": cur, "createdBy": pick(created), "needsReview": pick(review)})
		}
		ios := f.IO
		cs := ios.ColorScheme()
		out := ios.Out
		fmt.Fprintf(out, "\nRelevant pull requests in %s\n\n", r.FullName())
		if branch != "" {
			fmt.Fprintln(out, cs.Bold("Current branch"))
			if current != nil {
				printStatusLine(out, cs, current)
			} else {
				fmt.Fprintf(out, "  %s\n", cs.Gray(fmt.Sprintf("There is no pull request associated with %s", cs.Cyan("["+branch+"]"))))
			}
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, cs.Bold("Created by you"))
		if len(created) == 0 {
			fmt.Fprintf(out, "  %s\n", cs.Gray("You have no open pull requests"))
		}
		for _, p := range created {
			printStatusLine(out, cs, p)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("Requesting a code review from you"))
		if len(review) == 0 {
			fmt.Fprintf(out, "  %s\n", cs.Gray("You have no pull requests to review"))
		}
		for _, p := range review {
			printStatusLine(out, cs, p)
		}
		fmt.Fprintln(out)
		return nil
	}
	return c
}

func printStatusLine(out io.Writer, cs *iostreams.ColorScheme, p *shared.PullRequest) {
	color := shared.PullStateColor(cs, p.StateName(), p.Draft)
	fmt.Fprintf(out, "  %s  %s %s\n", color(fmt.Sprintf("#%d", p.Number)), p.Title, cs.Cyan("["+p.Head.Ref+"]"))
	state := shared.PullStateLabel(p)
	if state != "Open" {
		fmt.Fprintf(out, "    - %s\n", color(state))
	}
}
