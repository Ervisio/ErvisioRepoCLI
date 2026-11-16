package shared

import (
	"fmt"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

// Login is the login of u, "ghost" for a deleted user.
func Login(u *User) string {
	if u == nil {
		return "ghost"
	}
	return u.Login
}

// PrintBody renders a body or the "No description provided" note.
func PrintBody(ios *iostreams.IOStreams, body string) {
	cs := ios.ColorScheme()
	if strings.TrimSpace(body) == "" {
		fmt.Fprintf(ios.Out, "  %s\n", cs.Gray("No description provided"))
		return
	}
	fmt.Fprintln(ios.Out, cmdutil.RenderMarkdown(body, cs.Bold))
}

// PrintComments renders the conversation: all comments with --comments,
// else only the last one and a hint, as gh does.
func PrintComments(ios *iostreams.IOStreams, list []*Comment, all bool, count int) {
	if !all {
		if count > 0 {
			fmt.Fprintf(ios.Out, "\n%s\n", ios.ColorScheme().Gray(fmt.Sprintf("%d comment(s). Use --comments to view the full conversation", count)))
		}
		return
	}
	cs := ios.ColorScheme()
	for _, c := range list {
		fmt.Fprintln(ios.Out)
		fmt.Fprintf(ios.Out, "%s • %s\n", cs.Bold(Login(c.User)), cs.Gray(Ago(c.CreatedAt)))
		fmt.Fprintln(ios.Out, cmdutil.RenderMarkdown(c.Body, cs.Bold))
	}
}
