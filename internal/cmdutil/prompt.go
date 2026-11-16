package cmdutil

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

// Prompter asks questions on the terminal.
type Prompter struct{ IO *iostreams.IOStreams }

func (p *Prompter) ask(q string) (string, error) {
	cs := p.IO.ColorScheme()
	fmt.Fprintf(p.IO.ErrOut, "%s %s ", cs.Green("?"), cs.Bold(q))
	s, err := p.IO.ReadLine()
	if errors.Is(err, io.EOF) {
		return "", ErrCancel
	}
	if err != nil {
		return "", fmt.Errorf("read answer: %w", err)
	}
	return strings.TrimSpace(s), nil
}

// Input asks for a line of text.
func (p *Prompter) Input(q, def string) (string, error) {
	label := q
	if def != "" {
		label += " (" + def + ")"
	}
	s, err := p.ask(label)
	if err != nil {
		return "", err
	}
	if s == "" {
		return def, nil
	}
	return s, nil
}

// Password asks for a secret without echo.
func (p *Prompter) Password(q string) (string, error) {
	cs := p.IO.ColorScheme()
	fmt.Fprintf(p.IO.ErrOut, "%s %s ", cs.Green("?"), cs.Bold(q))
	s, err := p.IO.ReadPassword()
	if errors.Is(err, io.EOF) {
		return "", ErrCancel
	}
	if err != nil {
		return "", fmt.Errorf("read answer: %w", err)
	}
	return strings.TrimSpace(s), nil
}

// Select asks to choose an option and returns its index.
func (p *Prompter) Select(q string, def int, options []string) (int, error) {
	fmt.Fprintf(p.IO.ErrOut, "%s %s\n", p.IO.ColorScheme().Green("?"), p.IO.ColorScheme().Bold(q))
	for i, o := range options {
		fmt.Fprintf(p.IO.ErrOut, "  %d. %s\n", i+1, o)
	}
	for {
		s, err := p.ask(fmt.Sprintf("Choose 1-%d (%d):", len(options), def+1))
		if err != nil {
			return 0, err
		}
		if s == "" {
			return def, nil
		}
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		for i, o := range options {
			if strings.EqualFold(o, s) {
				return i, nil
			}
		}
		fmt.Fprintf(p.IO.ErrOut, "Invalid choice %q\n", s)
	}
}

// Confirm asks a yes/no question.
func (p *Prompter) Confirm(q string, def bool) (bool, error) {
	hint := "(y/N)"
	if def {
		hint = "(Y/n)"
	}
	for {
		s, err := p.ask(q + " " + hint)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(s) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}
