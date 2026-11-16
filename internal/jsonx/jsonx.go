// Package jsonx pretty-prints JSON with colors and evaluates the small
// subset of jq paths that eg supports.
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const (
	colorDelim = "1;38"
	colorKey   = "1;34"
	colorStr   = "32"
	colorNull  = "36"
	colorBool  = "33"
)

// Write pretty-prints the JSON document in r with two-space indentation,
// colored when color is set. Several concatenated documents are allowed.
func Write(w io.Writer, r io.Reader, color bool) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	p := &printer{w: &bytes.Buffer{}, color: color}
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("parse JSON: %w", err)
		}
		p.value(v, 0)
		p.w.WriteString("\n")
	}
	_, err := w.Write(p.w.Bytes())
	return err
}

// WriteValue pretty-prints a Go value.
func WriteValue(w io.Writer, v any, color bool) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return Write(w, bytes.NewReader(b), color)
}

type printer struct {
	w     *bytes.Buffer
	color bool
}

func (p *printer) paint(code, s string) {
	if p.color {
		p.w.WriteString("\x1b[" + code + "m" + s + "\x1b[m")
		return
	}
	p.w.WriteString(s)
}

func (p *printer) value(v any, depth int) {
	pad := strings.Repeat("  ", depth+1)
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			p.paint(colorDelim, "{}")
			return
		}
		p.paint(colorDelim, "{")
		p.w.WriteString("\n")
		keys := orderedKeys(x)
		for i, k := range keys {
			p.w.WriteString(pad)
			p.paint(colorKey, quote(k))
			p.paint(colorDelim, ":")
			p.w.WriteString(" ")
			p.value(x[k], depth+1)
			if i < len(keys)-1 {
				p.paint(colorDelim, ",")
			}
			p.w.WriteString("\n")
		}
		p.w.WriteString(strings.Repeat("  ", depth))
		p.paint(colorDelim, "}")
	case []any:
		if len(x) == 0 {
			p.paint(colorDelim, "[]")
			return
		}
		p.paint(colorDelim, "[")
		p.w.WriteString("\n")
		for i, e := range x {
			p.w.WriteString(pad)
			p.value(e, depth+1)
			if i < len(x)-1 {
				p.paint(colorDelim, ",")
			}
			p.w.WriteString("\n")
		}
		p.w.WriteString(strings.Repeat("  ", depth))
		p.paint(colorDelim, "]")
	case string:
		p.paint(colorStr, quote(x))
	case json.Number:
		p.w.WriteString(x.String())
	case bool:
		p.paint(colorBool, strconv.FormatBool(x))
	case nil:
		p.paint(colorNull, "null")
	default:
		b, _ := json.Marshal(x)
		p.w.Write(b)
	}
}

// orderedKeys sorts keys alphabetically, as jq and gh do.
func orderedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func quote(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// ErrUnsupportedJQ is returned for expressions beyond simple paths.
var ErrUnsupportedJQ = errors.New("eg supports only simple --jq paths such as '.', '.name', '.[].name', '.[0].owner.login' or '.items | .[]'")

// Query evaluates a jq path expression on the JSON in r and writes one
// result per line: strings raw, other values as compact JSON.
func Query(w io.Writer, r io.Reader, expr string) error {
	steps, err := parsePath(expr)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("parse JSON: %w", err)
		}
		results, err := eval([]any{v}, steps)
		if err != nil {
			return err
		}
		for _, res := range results {
			if s, ok := res.(string); ok {
				fmt.Fprintln(w, s)
				continue
			}
			var b bytes.Buffer
			enc := json.NewEncoder(&b)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(res); err != nil {
				return fmt.Errorf("encode JSON: %w", err)
			}
			w.Write(b.Bytes()) //nolint:errcheck // stdout
		}
	}
}

type step struct {
	key   string
	index int
	kind  int // 0 key, 1 index, 2 iterate
}

func parsePath(expr string) ([]step, error) {
	var steps []step
	for _, seg := range strings.Split(expr, "|") {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg[0] != '.' {
			return nil, ErrUnsupportedJQ
		}
		s := seg
		for len(s) > 0 {
			switch {
			case strings.HasPrefix(s, "[]"):
				steps = append(steps, step{kind: 2})
				s = s[2:]
			case strings.HasPrefix(s, "["):
				end := strings.IndexByte(s, ']')
				if end < 0 {
					return nil, ErrUnsupportedJQ
				}
				inner := s[1:end]
				if n, err := strconv.Atoi(inner); err == nil {
					steps = append(steps, step{kind: 1, index: n})
				} else if k, err := strconv.Unquote(inner); err == nil {
					steps = append(steps, step{kind: 0, key: k})
				} else {
					return nil, ErrUnsupportedJQ
				}
				s = s[end+1:]
			case s == ".":
				s = ""
			case strings.HasPrefix(s, ".["):
				s = s[1:]
			case strings.HasPrefix(s, "."):
				s = s[1:]
				end := strings.IndexAny(s, ".[")
				if end < 0 {
					end = len(s)
				}
				k := s[:end]
				if k == "" || strings.ContainsAny(k, " ()+-*/=<>,\"") {
					return nil, ErrUnsupportedJQ
				}
				steps = append(steps, step{kind: 0, key: k})
				s = s[end:]
			default:
				return nil, ErrUnsupportedJQ
			}
		}
	}
	return steps, nil
}

func eval(in []any, steps []step) ([]any, error) {
	cur := in
	for _, st := range steps {
		var next []any
		for _, v := range cur {
			switch st.kind {
			case 0:
				switch m := v.(type) {
				case map[string]any:
					next = append(next, m[st.key])
				case nil:
					next = append(next, nil)
				default:
					return nil, fmt.Errorf("cannot index %s with %q", typeName(v), st.key)
				}
			case 1:
				switch a := v.(type) {
				case []any:
					i := st.index
					if i < 0 {
						i += len(a)
					}
					if i < 0 || i >= len(a) {
						next = append(next, nil)
					} else {
						next = append(next, a[i])
					}
				case nil:
					next = append(next, nil)
				default:
					return nil, fmt.Errorf("cannot index %s with number", typeName(v))
				}
			case 2:
				switch a := v.(type) {
				case []any:
					next = append(next, a...)
				case map[string]any:
					for _, k := range orderedKeys(a) {
						next = append(next, a[k])
					}
				default:
					return nil, fmt.Errorf("cannot iterate over %s", typeName(v))
				}
			}
		}
		cur = next
	}
	return cur, nil
}

func typeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case nil:
		return "null"
	}
	return "number"
}
