package cmdutil

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// FlagSet wraps flag.FlagSet with gh-style long/short pairs, flags mixed
// with positional arguments and "--".
type FlagSet struct {
	fs      *flag.FlagSet
	defs    []*flagDef
	short   map[string]string // short -> long
	changed map[string]bool
}

type flagDef struct {
	name, short, usage string
	hidden             bool
}

// NewFlagSet returns an empty set.
func NewFlagSet(name string) *FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return &FlagSet{fs: fs, short: map[string]string{}, changed: map[string]bool{}}
}

func (f *FlagSet) add(name, short, usage string, bind func(n string)) {
	bind(name)
	if short != "" {
		bind(short)
		f.short[short] = name
	}
	f.defs = append(f.defs, &flagDef{name: name, short: short, usage: usage})
}

// StringVarP defines --name/-short.
func (f *FlagSet) StringVarP(p *string, name, short, def, usage string) {
	f.add(name, short, usage, func(n string) { f.fs.StringVar(p, n, def, usage) })
}

// BoolVarP defines a boolean flag.
func (f *FlagSet) BoolVarP(p *bool, name, short string, def bool, usage string) {
	f.add(name, short, usage, func(n string) { f.fs.BoolVar(p, n, def, usage) })
}

// IntVarP defines an integer flag.
func (f *FlagSet) IntVarP(p *int, name, short string, def int, usage string) {
	f.add(name, short, usage, func(n string) { f.fs.IntVar(p, n, def, usage) })
}

// StringArrayVarP defines a repeatable flag.
func (f *FlagSet) StringArrayVarP(p *[]string, name, short, usage string) {
	v := (*stringArray)(p)
	f.add(name, short, usage, func(n string) { f.fs.Var(v, n, usage) })
}

// EnumVarP defines a string flag restricted to values.
func (f *FlagSet) EnumVarP(p *string, name, short, def string, values []string, usage string) {
	*p = def
	v := &enumValue{p: p, values: values}
	usage += " {" + strings.Join(values, "|") + "}"
	f.add(name, short, usage, func(n string) { f.fs.Var(v, n, usage) })
}

// Hide keeps a flag out of the help text.
func (f *FlagSet) Hide(name string) {
	for _, d := range f.defs {
		if d.name == name {
			d.hidden = true
		}
	}
}

// Changed reports whether the flag was given.
func (f *FlagSet) Changed(name string) bool { return f.changed[name] }

// Lookup reports whether a flag (long or short) exists.
func (f *FlagSet) Lookup(name string) bool { return f.fs.Lookup(name) != nil }

type stringArray []string

func (s *stringArray) String() string     { return strings.Join(*s, ",") }
func (s *stringArray) Set(v string) error { *s = append(*s, v); return nil }

type enumValue struct {
	p      *string
	values []string
}

func (e *enumValue) String() string {
	if e.p == nil {
		return ""
	}
	return *e.p
}

func (e *enumValue) Set(v string) error {
	for _, x := range e.values {
		if strings.EqualFold(v, x) {
			*e.p = x
			return nil
		}
	}
	return fmt.Errorf("valid values are {%s}", strings.Join(e.values, "|"))
}

// Parse parses flags anywhere in args. It returns the positional
// arguments and those after "--".
func (f *FlagSet) Parse(args []string) (pos, dash []string, hasDash bool, err error) {
	for i, a := range args {
		if a == "--" {
			args, dash, hasDash = args[:i], args[i+1:], true
			break
		}
	}
	for {
		if err := f.fs.Parse(args); err != nil {
			return nil, nil, false, f.translate(err)
		}
		rest := f.fs.Args()
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
	f.fs.Visit(func(fl *flag.Flag) {
		name := fl.Name
		if long, ok := f.short[name]; ok {
			name = long
		}
		f.changed[name] = true
	})
	return pos, dash, hasDash, nil
}

// translate rewrites the errors of package flag in the pflag wording gh
// users know.
func (f *FlagSet) translate(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "flag provided but not defined: "):
		name := strings.TrimPrefix(msg, "flag provided but not defined: ")
		return fmt.Errorf("unknown flag: %s", dashed(strings.TrimLeft(name, "-")))
	case strings.HasPrefix(msg, "flag needs an argument: "):
		name := strings.TrimLeft(strings.TrimPrefix(msg, "flag needs an argument: "), "-")
		return &missingArgError{name: f.longName(name)}
	case strings.HasPrefix(msg, "invalid value "):
		return errors.New(strings.Replace(msg, " for flag -", " for flag --", 1))
	}
	return err
}

func (f *FlagSet) longName(n string) string {
	if long, ok := f.short[n]; ok {
		return long
	}
	return n
}

func dashed(n string) string {
	if len(n) == 1 {
		return "-" + n
	}
	return "--" + n
}

type missingArgError struct{ name string }

func (e *missingArgError) Error() string { return "flag needs an argument: --" + e.name }

// usages renders the flag list as pflag does.
func (f *FlagSet) usages() string {
	type row struct{ left, usage string }
	var rows []row
	width := 0
	for _, d := range f.defs {
		if d.hidden {
			continue
		}
		fl := f.fs.Lookup(d.name)
		typ, usage := flag.UnquoteUsage(fl)
		if _, ok := fl.Value.(*stringArray); ok && typ == "value" {
			typ = "stringArray"
		}
		if _, ok := fl.Value.(*enumValue); ok && typ == "value" {
			typ = "string"
		}
		left := "    "
		if d.short != "" {
			left = "-" + d.short + ", "
		}
		left += "--" + d.name
		if typ != "" {
			left += " " + typ
		}
		if def := fl.DefValue; def != "" && def != "false" && def != "0" && def != "[]" {
			if typ != "int" {
				def = strconv.Quote(def)
			}
			usage += " (default " + def + ")"
		}
		rows = append(rows, row{left, usage})
		width = max(width, len(left))
	}
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "  %-*s   %s\n", width, r.left, r.usage)
	}
	return b.String()
}

// Set assigns a flag value and marks it as changed, as if it had been
// given on the command line.
func (f *FlagSet) Set(name, value string) error {
	if err := f.fs.Set(name, value); err != nil {
		return fmt.Errorf("set --%s: %w", name, err)
	}
	f.changed[f.longName(name)] = true
	return nil
}

// FlagNames lists the visible flags as "--name" (and "-s" for shorts).
func (f *FlagSet) FlagNames() []string {
	var out []string
	for _, d := range f.defs {
		if d.hidden {
			continue
		}
		out = append(out, "--"+d.name)
		if d.short != "" {
			out = append(out, "-"+d.short)
		}
	}
	return out
}
