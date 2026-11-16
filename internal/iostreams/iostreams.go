// Package iostreams wraps the standard streams with TTY and color
// detection.
package iostreams

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// IOStreams are the streams of a command.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	inFile    *os.File
	stdinTTY  bool
	stdoutTTY bool
	stderrTTY bool
	color     bool
	noPrompt  bool
}

// System returns the process streams.
func System() *IOStreams {
	s := &IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr, inFile: os.Stdin}
	s.stdinTTY = isTerminal(os.Stdin)
	s.stdoutTTY = isTerminal(os.Stdout)
	s.stderrTTY = isTerminal(os.Stderr)
	if v := os.Getenv("EG_FORCE_TTY"); v != "" && !isFalse(v) {
		s.stdoutTTY, s.stderrTTY = true, true
	}
	s.color = s.stdoutTTY && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	if os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0" {
		s.color = true
	}
	s.noPrompt = os.Getenv("EG_PROMPT_DISABLED") != ""
	return s
}

func isFalse(v string) bool {
	b, err := strconv.ParseBool(v)
	return err == nil && !b
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// IsStdinTTY reports whether stdin is a terminal.
func (s *IOStreams) IsStdinTTY() bool { return s.stdinTTY }

// IsStdoutTTY reports whether stdout is a terminal.
func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutTTY }

// IsStderrTTY reports whether stderr is a terminal.
func (s *IOStreams) IsStderrTTY() bool { return s.stderrTTY }

// CanPrompt reports whether interactive prompts may be shown.
func (s *IOStreams) CanPrompt() bool { return s.stdinTTY && s.stdoutTTY && !s.noPrompt }

// ColorEnabled reports whether stdout output is colored.
func (s *IOStreams) ColorEnabled() bool { return s.color }

// TerminalWidth is the width of stdout, 80 when unknown.
func (s *IOStreams) TerminalWidth() int {
	if w, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && w > 0 {
		return w
	}
	if w := termWidth(os.Stdout); w > 0 {
		return w
	}
	return 80
}

// ReadPassword reads a line from stdin without echoing it when stdin is
// a terminal.
func (s *IOStreams) ReadPassword() (string, error) {
	if s.inFile != nil && s.stdinTTY {
		if restore, err := disableEcho(s.inFile); err == nil {
			defer restore()
			defer io.WriteString(s.ErrOut, "\n") //nolint:errcheck // cosmetic newline
		}
	}
	return readLine(s.In)
}

// ReadLine reads one line from stdin without the trailing newline.
func (s *IOStreams) ReadLine() (string, error) { return readLine(s.In) }

func readLine(r io.Reader) (string, error) {
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				break
			}
			buf = append(buf, b[0])
		}
		if err == io.EOF {
			if len(buf) == 0 {
				return "", io.EOF
			}
			break
		}
		if err != nil {
			return "", err
		}
	}
	return string(bytes.TrimRight(buf, "\r")), nil
}

// ColorScheme returns the color helpers of stdout.
func (s *IOStreams) ColorScheme() *ColorScheme {
	ct := os.Getenv("COLORTERM")
	return &ColorScheme{enabled: s.color, trueColor: s.color && (strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit"))}
}

// ColorScheme applies ANSI colors when enabled.
type ColorScheme struct{ enabled, trueColor bool }

// HexColor paints t with an "rrggbb" color on true-color terminals.
func (c *ColorScheme) HexColor(hex, t string) string {
	hex = strings.TrimPrefix(hex, "#")
	if !c.trueColor || len(hex) != 6 {
		return t
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return t
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, t)
}

func (c *ColorScheme) wrap(code, t string) string {
	if !c.enabled || t == "" {
		return t
	}
	return "\x1b[" + code + "m" + t + "\x1b[0m"
}

// Enabled reports whether colors are on.
func (c *ColorScheme) Enabled() bool { return c.enabled }

func (c *ColorScheme) Bold(t string) string    { return c.wrap("1", t) }
func (c *ColorScheme) Red(t string) string     { return c.wrap("31", t) }
func (c *ColorScheme) Green(t string) string   { return c.wrap("32", t) }
func (c *ColorScheme) Yellow(t string) string  { return c.wrap("33", t) }
func (c *ColorScheme) Blue(t string) string    { return c.wrap("34", t) }
func (c *ColorScheme) Magenta(t string) string { return c.wrap("35", t) }
func (c *ColorScheme) Cyan(t string) string    { return c.wrap("36", t) }
func (c *ColorScheme) Gray(t string) string    { return c.wrap("90", t) }

// SuccessIcon is a green check mark.
func (c *ColorScheme) SuccessIcon() string { return c.Green("✓") }

// FailureIcon is a red X.
func (c *ColorScheme) FailureIcon() string { return c.Red("X") }

// WarningIcon is a yellow exclamation mark.
func (c *ColorScheme) WarningIcon() string { return c.Yellow("!") }
