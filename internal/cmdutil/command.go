package cmdutil

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

// Command is a node of the command tree.
type Command struct {
	// Use is the name followed by the argument synopsis.
	Use     string
	Short   string
	Long    string
	Example string
	Aliases []string
	// GroupID places the command in a help section of its parent.
	GroupID string
	Hidden  bool
	Flags   *FlagSet
	// Args validates the positional arguments; nil accepts any.
	Args func(args []string) error
	// Run executes a leaf command.
	Run func(c *Command, args []string) error
	// KeepDash keeps the arguments after "--" in DashArgs instead of
	// treating them as positional.
	KeepDash bool
	DashArgs []string
	HasDash  bool
	// JSONFields lists the fields accepted by --json.
	JSONFields []string
	// IO receives help output; children inherit it.
	IO *iostreams.IOStreams

	parent *Command
	subs   []*Command
	groups []Group
}

// Group is a help section of subcommands.
type Group struct{ ID, Title string }

// AddGroup declares help sections, in order.
func (c *Command) AddGroup(g ...Group) { c.groups = append(c.groups, g...) }

// AddCommand attaches subcommands.
func (c *Command) AddCommand(subs ...*Command) {
	for _, s := range subs {
		s.parent = c
		c.subs = append(c.subs, s)
	}
}

// Name is the first word of Use.
func (c *Command) Name() string {
	n, _, _ := strings.Cut(c.Use, " ")
	return n
}

// CommandPath is the full command, "eg repo list".
func (c *Command) CommandPath() string {
	if c.parent == nil {
		return c.Name()
	}
	return c.parent.CommandPath() + " " + c.Name()
}

// Root returns the top of the tree.
func (c *Command) Root() *Command {
	for c.parent != nil {
		c = c.parent
	}
	return c
}

func (c *Command) io() *iostreams.IOStreams {
	for x := c; x != nil; x = x.parent {
		if x.IO != nil {
			return x.IO
		}
	}
	return iostreams.System()
}

// Sub finds a subcommand by name or alias.
func (c *Command) Sub(name string) *Command {
	for _, s := range c.subs {
		if s.Name() == name {
			return s
		}
		for _, a := range s.Aliases {
			if a == name {
				return s
			}
		}
	}
	return nil
}

// Execute runs the command addressed by args.
func (c *Command) Execute(args []string) error {
	if c.Run == nil {
		if len(args) == 0 {
			c.PrintHelp()
			return nil
		}
		switch args[0] {
		case "--help", "-h", "-help":
			c.PrintHelp()
			return nil
		}
		sub := c.Sub(args[0])
		if sub == nil {
			if strings.HasPrefix(args[0], "-") {
				return &FlagError{Err: fmt.Errorf("unknown flag: %s", args[0]), Cmd: c}
			}
			return &FlagError{Err: fmt.Errorf("unknown command %q for %q", args[0], c.CommandPath()), Cmd: c}
		}
		return sub.Execute(args[1:])
	}
	if c.Flags == nil {
		c.Flags = NewFlagSet(c.Name())
	}
	var help bool
	if !c.Flags.Lookup("help") {
		c.Flags.BoolVarP(&help, "help", "", false, "Show help for command")
		c.Flags.Hide("help")
	}
	pos, dash, hasDash, err := c.Flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			c.PrintHelp()
			return nil
		}
		var ma *missingArgError
		if errors.As(err, &ma) && ma.name == "json" && len(c.JSONFields) > 0 {
			return jsonFieldsError(c.JSONFields)
		}
		return &FlagError{Err: err, Cmd: c}
	}
	if help {
		c.PrintHelp()
		return nil
	}
	c.HasDash = hasDash
	if c.KeepDash {
		c.DashArgs = dash
	} else {
		pos = append(pos, dash...)
	}
	if c.Args != nil {
		if err := c.Args(pos); err != nil {
			return &FlagError{Err: err, Cmd: c}
		}
	}
	err = c.Run(c, pos)
	var fe *FlagError
	if errors.As(err, &fe) && fe.Cmd == nil {
		fe.Cmd = c
	}
	return err
}

func jsonFieldsError(fields []string) error {
	sorted := append([]string(nil), fields...)
	sort.Strings(sorted)
	var b strings.Builder
	b.WriteString("Specify one or more comma-separated fields for `--json`:")
	for _, f := range sorted {
		b.WriteString("\n  " + f)
	}
	return errors.New(b.String())
}

// ---- argument validators ----

// NoArgs refuses positional arguments.
func NoArgs(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("accepts 0 arg(s), received %d", len(args))
	}
	return nil
}

// MaxArgs accepts at most n arguments.
func MaxArgs(n int) func([]string) error {
	return func(args []string) error {
		if len(args) > n {
			return fmt.Errorf("accepts at most %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}

// ExactArgs requires n arguments; msg replaces the generic message when
// arguments are missing.
func ExactArgs(n int, msg string) func([]string) error {
	return func(args []string) error {
		if len(args) < n && msg != "" {
			return errors.New(msg)
		}
		if len(args) != n {
			return fmt.Errorf("accepts %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}

// RangeArgs accepts between lo and hi arguments.
func RangeArgs(lo, hi int, msg string) func([]string) error {
	return func(args []string) error {
		if len(args) < lo {
			if msg != "" {
				return errors.New(msg)
			}
			return fmt.Errorf("accepts between %d and %d arg(s), received %d", lo, hi, len(args))
		}
		if len(args) > hi {
			return fmt.Errorf("accepts at most %d arg(s), received %d", hi, len(args))
		}
		return nil
	}
}

// ---- help ----

// UsageLine is "eg repo list [<owner>] [flags]".
func (c *Command) UsageLine() string {
	if c.Run == nil {
		if c.parent == nil {
			return c.Name() + " <command> <subcommand> [flags]"
		}
		return c.CommandPath() + " <command> [flags]"
	}
	use := c.CommandPath()
	if _, rest, ok := strings.Cut(c.Use, " "); ok {
		use += " " + rest
	}
	if c.Flags != nil && len(c.Flags.defs) > 0 && !strings.Contains(use, "[flags]") {
		use += " [flags]"
	}
	return use
}

// UsageString is the short usage printed after a flag error.
func (c *Command) UsageString() string {
	var b strings.Builder
	b.WriteString("Usage:  " + c.UsageLine() + "\n")
	if c.Flags != nil {
		if u := c.Flags.usages(); u != "" {
			b.WriteString("\nFlags:\n" + u)
		}
	}
	return b.String()
}

// PrintHelp writes the full help to stdout.
func (c *Command) PrintHelp() { c.WriteHelp(c.io().Out) }

// WriteHelp renders the help in gh's layout.
func (c *Command) WriteHelp(w io.Writer) {
	cs := c.io().ColorScheme()
	section := func(title, body string) {
		if strings.TrimSpace(body) == "" {
			return
		}
		fmt.Fprintf(w, "%s\n%s\n", cs.Bold(title), strings.TrimRight(body, "\n")+"\n")
	}
	desc := c.Long
	if desc == "" {
		desc = c.Short
	}
	fmt.Fprintln(w, strings.TrimSpace(desc))
	fmt.Fprintln(w)
	section("USAGE", "  "+c.UsageLine())
	if len(c.Aliases) > 0 {
		var al []string
		for _, a := range c.Aliases {
			al = append(al, "  "+c.parent.CommandPath()+" "+a)
		}
		section("ALIASES", strings.Join(al, "\n"))
	}
	if c.Run == nil {
		width := 0
		for _, s := range c.subs {
			width = max(width, len(s.Name())+1)
		}
		list := func(group string) string {
			var b strings.Builder
			for _, s := range c.subs {
				if s.Hidden || s.GroupID != group {
					continue
				}
				fmt.Fprintf(&b, "  %-*s  %s\n", width, s.Name()+":", s.Short)
			}
			return b.String()
		}
		for _, g := range c.groups {
			section(g.Title, list(g.ID))
		}
		title := "AVAILABLE COMMANDS"
		if len(c.groups) > 0 {
			title = "ADDITIONAL COMMANDS"
		}
		section(title, list(""))
	}
	if c.Flags != nil {
		section("FLAGS", c.Flags.usages())
	}
	if c.parent == nil {
		section("FLAGS", "  --help      Show help for command\n  --version   Show eg version")
	} else {
		section("INHERITED FLAGS", "  --help   Show help for command")
	}
	if c.Example != "" {
		var b strings.Builder
		for _, l := range strings.Split(strings.Trim(c.Example, "\n"), "\n") {
			if l != "" {
				l = "  " + l
			}
			b.WriteString(l + "\n")
		}
		section("EXAMPLES", b.String())
	}
	if c.parent == nil {
		section("ENVIRONMENT VARIABLES", "  See 'eg help environment' for info on environment variables that you can set.")
	}
	section("LEARN MORE", "  Use `eg <command> <subcommand> --help` for more information about a command.\n"+
		"  Read the manual at https://github.com/Ervisio/ErvisioRepoCLI#readme")
}
