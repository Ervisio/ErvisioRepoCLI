package cmdutil

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

// Table prints aligned, truncated columns on a terminal and
// tab-separated values otherwise, as gh's tableprinter.
type Table struct {
	out    io.Writer
	bold   func(string) string
	tty    bool
	width  int
	header []string
	rows   [][]cell
	row    []cell
}

type cell struct {
	text     string
	color    func(string) string
	truncate bool
}

// NewTable returns a printer for the command output.
func NewTable(ios *iostreams.IOStreams) *Table {
	return &Table{out: ios.Out, bold: ios.ColorScheme().Bold, tty: ios.IsStdoutTTY(), width: ios.TerminalWidth()}
}

// IsTTY reports whether the table renders for a terminal.
func (t *Table) IsTTY() bool { return t.tty }

// Header sets the column titles; they are printed on terminals only.
func (t *Table) Header(cols ...string) {
	t.header = make([]string, len(cols))
	for i, c := range cols {
		t.header[i] = strings.ToUpper(c)
	}
}

// Field adds a cell; color may be nil. Truncatable cells shrink first
// when the row is wider than the terminal.
func (t *Table) Field(text string, color func(string) string, truncate bool) {
	t.row = append(t.row, cell{text: text, color: color, truncate: truncate})
}

// EndRow closes the current row.
func (t *Table) EndRow() {
	t.rows = append(t.rows, t.row)
	t.row = nil
}

// Render writes the table.
func (t *Table) Render() error {
	if len(t.row) > 0 {
		t.EndRow()
	}
	if !t.tty {
		for _, r := range t.rows {
			parts := make([]string, len(r))
			for i, c := range r {
				parts[i] = c.text
			}
			if _, err := fmt.Fprintln(t.out, strings.Join(parts, "\t")); err != nil {
				return err
			}
		}
		return nil
	}
	rows := t.rows
	if len(t.header) > 0 && len(rows) > 0 {
		h := make([]cell, len(t.header))
		for i, s := range t.header {
			h[i] = cell{text: s, color: t.bold, truncate: true}
		}
		rows = append([][]cell{h}, rows...)
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	widths := make([]int, cols)
	truncatable := make([]bool, cols)
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], displayWidth(c.text))
			if c.truncate {
				truncatable[i] = true
			}
		}
	}
	const sep = 2
	total := func() int {
		n := sep * (cols - 1)
		for _, w := range widths {
			n += w
		}
		return n
	}
	for total() > t.width {
		widest := -1
		for i := range widths {
			if truncatable[i] && widths[i] > 5 && (widest < 0 || widths[i] > widths[widest]) {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		widths[widest] -= min(total()-t.width, widths[widest]-5)
	}
	for _, r := range rows {
		var b strings.Builder
		for i, c := range r {
			text := truncateText(c.text, widths[i])
			pad := widths[i] - displayWidth(text)
			if c.color != nil {
				text = c.color(text)
			}
			b.WriteString(text)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", pad+sep))
			}
		}
		if _, err := fmt.Fprintln(t.out, b.String()); err != nil {
			return err
		}
	}
	return nil
}

func displayWidth(s string) int { return utf8.RuneCountInString(s) }

func truncateText(s string, w int) string {
	if displayWidth(s) <= w {
		return s
	}
	if w <= 3 {
		return string([]rune(s)[:w])
	}
	return string([]rune(s)[:w-3]) + "..."
}

// FuzzyAgo describes a past time as gh does: "about 2 days ago".
func FuzzyAgo(now, t time.Time) string {
	d := now.Sub(t)
	unit := func(n int, u string) string {
		if n != 1 {
			u += "s"
		}
		return fmt.Sprintf("about %d %s ago", n, u)
	}
	switch {
	case d < time.Minute:
		return "less than a minute ago"
	case d < time.Hour:
		return unit(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return unit(int(d.Hours()), "hour")
	case d < 30*24*time.Hour:
		return unit(int(d.Hours()/24), "day")
	case d < 365*24*time.Hour:
		return unit(int(d.Hours()/24/30), "month")
	}
	return unit(int(d.Hours()/24/365), "year")
}
