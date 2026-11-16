package cmdutil

import "strings"

// RenderMarkdown prints Markdown plainly: headings in bold, everything
// else as written.
func RenderMarkdown(md string, bold func(string) string) string {
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
