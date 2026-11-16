package issue

import (
	"fmt"
	"io"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

func newView(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag      string
		web, comments bool
		exporter      cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "view {<number> | <url>}",
		Short: "View an issue",
		Long: `Display the title, body, and other information about an issue.

With '--web', open the issue in a web browser instead.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("view"),
		Args:    cmdutil.ExactArgs(1, "argument required: issue number or URL"),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&web, "web", "w", false, "Open an issue in the browser")
	c.Flags.BoolVarP(&comments, "comments", "c", false, "View issue comments")
	cmdutil.AddJSONFlags(c, &exporter, shared.IssueJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		r, n, err := shared.Target(f, repoFlag, args[0])
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		if web {
			return f.OpenURL(f.WebURL(r, fmt.Sprintf("issues/%d", n)))
		}
		issue, err := shared.FetchIssue(client, r, n)
		if err != nil {
			return err
		}
		if issue.PullRequest != nil {
			return fmt.Errorf("#%d is a pull request; use `eg pr view %d`", n, n)
		}
		if exporter.Enabled() {
			m := issue.ExportFields()
			if shared.WantsField(&exporter, "comments") {
				if m["comments"], err = shared.ExportComments(client, r, n); err != nil {
					return err
				}
			}
			return exporter.Write(f.IO, exporter.Pick(m))
		}
		var list []*shared.Comment
		if comments {
			if list, err = shared.ListComments(client, r, n); err != nil {
				return err
			}
		}
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(f.IO.ErrOut, "failed to start pager: %v\n", err)
		}
		defer f.StopPager()
		if !f.IO.IsStdoutTTY() {
			printRawIssue(f.IO.Out, issue, list, comments)
			return nil
		}
		printIssue(f.IO, issue, list, comments)
		return nil
	}
	return c
}

func printRawIssue(out io.Writer, i *shared.Issue, list []*shared.Comment, comments bool) {
	if comments {
		for _, c := range list {
			fmt.Fprintf(out, "author:\t%s\n", shared.Login(c.User))
			fmt.Fprintf(out, "association:\t%s\n", strings.ToLower(c.AuthorAssociation))
			fmt.Fprintf(out, "edited:\t%v\n", c.UpdatedAt != c.CreatedAt)
			fmt.Fprintln(out, "--")
			fmt.Fprintln(out, c.Body)
			fmt.Fprintln(out, "--")
		}
		return
	}
	milestone := ""
	if i.Milestone != nil {
		milestone = i.Milestone.Title
	}
	fmt.Fprintf(out, "title:\t%s\n", i.Title)
	fmt.Fprintf(out, "state:\t%s\n", strings.ToUpper(i.State))
	fmt.Fprintf(out, "author:\t%s\n", shared.Login(i.User))
	fmt.Fprintf(out, "labels:\t%s\n", strings.Join(shared.LabelNames(i.Labels), ", "))
	fmt.Fprintf(out, "comments:\t%d\n", i.Comments)
	fmt.Fprintf(out, "assignees:\t%s\n", strings.Join(shared.Logins(i.Assignees), ", "))
	fmt.Fprintf(out, "projects:\t\n")
	fmt.Fprintf(out, "milestone:\t%s\n", milestone)
	fmt.Fprintf(out, "number:\t%d\n", i.Number)
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, i.BodyText())
}

func stateTitle(i *shared.Issue) string {
	if i.State == "open" {
		return "Open"
	}
	if i.Reason() == "not_planned" {
		return "Closed as not planned"
	}
	return "Closed"
}

func printIssue(ios *iostreams.IOStreams, i *shared.Issue, list []*shared.Comment, comments bool) {
	cs := ios.ColorScheme()
	out := ios.Out
	fmt.Fprintf(out, "%s %s\n", cs.Bold(i.Title), cs.Gray(fmt.Sprintf("#%d", i.Number)))
	color := shared.IssueStateColor(cs, i.State, i.Reason())
	noun := "comments"
	if i.Comments == 1 {
		noun = "comment"
	}
	fmt.Fprintf(out, "%s • %s opened %s • %d %s\n", color(stateTitle(i)), shared.Login(i.User), shared.Ago(i.CreatedAt), i.Comments, noun)
	if a := shared.Logins(i.Assignees); len(a) > 0 {
		fmt.Fprintf(out, "%s %s\n", cs.Bold("Assignees:"), strings.Join(a, ", "))
	}
	if len(i.Labels) > 0 {
		fmt.Fprintf(out, "%s %s\n", cs.Bold("Labels:"), strings.Join(shared.LabelNames(i.Labels), ", "))
	}
	if i.Milestone != nil {
		fmt.Fprintf(out, "%s %s\n", cs.Bold("Milestone:"), i.Milestone.Title)
	}
	fmt.Fprintln(out)
	shared.PrintBody(ios, i.BodyText())
	shared.PrintComments(ios, list, comments, i.Comments)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", cs.Gray("View this issue on ErvisioRepo: "+i.HTMLURL))
}
