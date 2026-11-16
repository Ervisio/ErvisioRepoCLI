package pr

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
		Use:   "view " + argUse,
		Short: "View a pull request",
		Long: `Display the title, body, and other information about a pull request.

Without an argument, the pull request that belongs to the current branch
is displayed.

With '--web', open the pull request in a web browser instead.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("view"),
		Args:    selectorArgs,
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&web, "web", "w", false, "Open a pull request in the browser")
	c.Flags.BoolVarP(&comments, "comments", "c", false, "View pull request comments")
	cmdutil.AddJSONFlags(c, &exporter, shared.PullJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if err := exporter.Validate(); err != nil {
			return err
		}
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		if web {
			return f.OpenURL(p.HTMLURL)
		}
		if exporter.Enabled() {
			m := p.ExportFields()
			if shared.WantsField(&exporter, "comments") {
				if m["comments"], err = shared.ExportComments(fd.client, fd.repo, p.Number); err != nil {
					return err
				}
			}
			return exporter.Write(f.IO, exporter.Pick(m))
		}
		var list []*shared.Comment
		if comments {
			if list, err = shared.ListComments(fd.client, fd.repo, p.Number); err != nil {
				return err
			}
		}
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(f.IO.ErrOut, "failed to start pager: %v\n", err)
		}
		defer f.StopPager()
		if !f.IO.IsStdoutTTY() {
			printRawPull(f.IO.Out, p, list, comments)
			return nil
		}
		printPull(f.IO, p, list, comments)
		return nil
	}
	return c
}

func printRawPull(out io.Writer, p *shared.PullRequest, list []*shared.Comment, comments bool) {
	if comments {
		for _, c := range list {
			fmt.Fprintf(out, "author:\t%s\nassociation:\t%s\nedited:\t%v\nstatus:\tnone\n--\n%s\n--\n",
				shared.Login(c.User), strings.ToLower(c.AuthorAssociation), c.UpdatedAt != c.CreatedAt, c.Body)
		}
		return
	}
	milestone := ""
	if p.Milestone != nil {
		milestone = p.Milestone.Title
	}
	auto := "disabled"
	if len(p.AutoMerge) > 0 {
		auto = "enabled"
		if m, ok := p.AutoMerge["merge_method"].(string); ok {
			auto += "\t" + m
		}
	}
	state := p.StateName()
	if p.Draft && state == "OPEN" {
		state = "DRAFT"
	}
	fmt.Fprintf(out, "title:\t%s\n", p.Title)
	fmt.Fprintf(out, "state:\t%s\n", state)
	fmt.Fprintf(out, "author:\t%s\n", shared.Login(p.User))
	fmt.Fprintf(out, "labels:\t%s\n", strings.Join(shared.LabelNames(p.Labels), ", "))
	fmt.Fprintf(out, "assignees:\t%s\n", strings.Join(shared.Logins(p.Assignees), ", "))
	fmt.Fprintf(out, "reviewers:\t%s\n", strings.Join(shared.Logins(p.RequestedReviewers), ", "))
	fmt.Fprintf(out, "projects:\t\n")
	fmt.Fprintf(out, "milestone:\t%s\n", milestone)
	fmt.Fprintf(out, "number:\t%d\n", p.Number)
	fmt.Fprintf(out, "url:\t%s\n", p.HTMLURL)
	fmt.Fprintf(out, "additions:\t%d\n", p.Additions)
	fmt.Fprintf(out, "deletions:\t%d\n", p.Deletions)
	fmt.Fprintf(out, "auto-merge:\t%s\n", auto)
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, p.BodyText())
}

func printPull(ios *iostreams.IOStreams, p *shared.PullRequest, list []*shared.Comment, comments bool) {
	cs := ios.ColorScheme()
	out := ios.Out
	fmt.Fprintf(out, "%s %s\n", cs.Bold(p.Title), cs.Gray(fmt.Sprintf("#%d", p.Number)))
	color := shared.PullStateColor(cs, p.StateName(), p.Draft)
	noun := "commits"
	if p.Commits == 1 {
		noun = "commit"
	}
	verb := "wants to merge"
	if p.StateName() == "MERGED" {
		verb = "merged"
	}
	fmt.Fprintf(out, "%s • %s %s %d %s into %s from %s • %s\n", color(shared.PullStateLabel(p)), shared.Login(p.User),
		verb, p.Commits, noun, cs.Cyan(p.Base.Ref), cs.Cyan(headLabel(p)), cs.Gray(shared.Ago(p.CreatedAt)))
	fmt.Fprintf(out, "%s %s\n", cs.Green(fmt.Sprintf("+%d", p.Additions)), cs.Red(fmt.Sprintf("-%d", p.Deletions)))
	if len(p.AutoMerge) > 0 {
		m, _ := p.AutoMerge["merge_method"].(string)
		fmt.Fprintf(out, "%s %s\n", cs.Bold("Auto-merge:"), cs.Green("enabled")+" "+m)
	}
	if r := shared.Logins(p.RequestedReviewers); len(r) > 0 {
		fmt.Fprintf(out, "%s %s (Requested)\n", cs.Bold("Reviewers:"), strings.Join(r, " (Requested), "))
	}
	if a := shared.Logins(p.Assignees); len(a) > 0 {
		fmt.Fprintf(out, "%s %s\n", cs.Bold("Assignees:"), strings.Join(a, ", "))
	}
	if len(p.Labels) > 0 {
		fmt.Fprintf(out, "%s %s\n", cs.Bold("Labels:"), strings.Join(shared.LabelNames(p.Labels), ", "))
	}
	if p.Milestone != nil {
		fmt.Fprintf(out, "%s %s\n", cs.Bold("Milestone:"), p.Milestone.Title)
	}
	fmt.Fprintln(out)
	shared.PrintBody(ios, p.BodyText())
	shared.PrintComments(ios, list, comments, p.Comments)
	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Gray("View this pull request on ErvisioRepo: "+p.HTMLURL))
}

// headLabel is "branch" or "owner:branch" for a fork.
func headLabel(p *shared.PullRequest) string {
	if p.IsCrossRepository() && p.Head.Repo != nil {
		return p.Head.Repo.Owner.Login + ":" + p.Head.Ref
	}
	return p.Head.Ref
}
