package release

import (
	"fmt"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

func newView(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag string
		web      bool
		exporter cmdutil.Exporter
	)
	c := &cmdutil.Command{
		Use:   "view [<tag>]",
		Short: "View information about a release",
		Long: `View information about an ErvisioRepo release.

Without an explicit tag name argument, the latest release in the project
is shown.`,
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("view"),
		Args:    cmdutil.MaxArgs(1),
	}
	addRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&web, "web", "w", false, "Open the release in the browser")
	cmdutil.AddJSONFlags(c, &exporter, releaseJSONFields)
	c.Run = func(_ *cmdutil.Command, args []string) error {
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
		tag := ""
		if len(args) > 0 {
			tag = args[0]
		}
		rel, err := findRelease(client, r, tag)
		if err != nil {
			return err
		}
		if web {
			return f.OpenURL(rel.HTMLURL)
		}
		if exporter.Enabled() {
			return exporter.Write(f.IO, exporter.Pick(rel.export()))
		}
		ios := f.IO
		out := ios.Out
		if !ios.IsStdoutTTY() {
			fmt.Fprintf(out, "title:\t%s\n", deref(rel.Name))
			fmt.Fprintf(out, "tag:\t%s\n", rel.TagName)
			fmt.Fprintf(out, "draft:\t%v\n", rel.Draft)
			fmt.Fprintf(out, "prerelease:\t%v\n", rel.Prerelease)
			fmt.Fprintf(out, "author:\t%s\n", shared.Login(rel.Author))
			fmt.Fprintf(out, "created:\t%s\n", rel.CreatedAt)
			if !rel.Draft {
				fmt.Fprintf(out, "published:\t%s\n", deref(rel.PublishedAt))
			}
			fmt.Fprintf(out, "url:\t%s\n", rel.HTMLURL)
			for _, a := range rel.Assets {
				fmt.Fprintf(out, "asset:\t%s\n", a.Name)
			}
			fmt.Fprintln(out, "--")
			fmt.Fprintln(out, rel.body())
			return nil
		}
		if err := f.StartPager(); err != nil {
			fmt.Fprintf(ios.ErrOut, "failed to start pager: %v\n", err)
		}
		defer f.StopPager()
		cs := ios.ColorScheme()
		out = ios.Out
		fmt.Fprintln(out, cs.Bold(rel.TagName))
		switch {
		case rel.Draft:
			fmt.Fprintf(out, "%s • %s\n", cs.Red("Draft"), cs.Gray(fmt.Sprintf("%s created this %s", shared.Login(rel.Author), shared.Ago(rel.CreatedAt))))
		default:
			if rel.Prerelease {
				fmt.Fprintf(out, "%s • ", cs.Yellow("Pre-release"))
			}
			fmt.Fprintln(out, cs.Gray(fmt.Sprintf("%s released this %s", shared.Login(rel.Author), shared.Ago(deref(rel.PublishedAt)))))
		}
		fmt.Fprintln(out)
		shared.PrintBody(ios, rel.body())
		if len(rel.Assets) > 0 {
			fmt.Fprintf(out, "\n%s\n", cs.Bold("Assets"))
			t := cmdutil.NewTable(ios)
			for _, a := range rel.Assets {
				t.Field(a.Name, nil, true)
				t.Field(humanSize(a.Size), nil, false)
				t.EndRow()
			}
			if err := t.Render(); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "\n%s\n", cs.Gray("View on ErvisioRepo: "+rel.HTMLURL))
		return nil
	}
	return c
}
