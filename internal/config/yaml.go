package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Node is a YAML node of the small subset used by hosts.yml and
// config.yml: nested mappings of string scalars. Key order is kept so a
// rewritten file stays close to the original.
type Node struct {
	keys   []string
	values map[string]*Node
	// Scalar is the value of a leaf node.
	Scalar string
	isMap  bool
}

// NewMap returns an empty mapping node.
func NewMap() *Node { return &Node{isMap: true, values: map[string]*Node{}} }

// IsMap reports whether n is a mapping.
func (n *Node) IsMap() bool { return n != nil && n.isMap }

// Keys returns the keys of a mapping in file order.
func (n *Node) Keys() []string {
	if !n.IsMap() {
		return nil
	}
	return append([]string(nil), n.keys...)
}

// Get returns the child at key, or nil.
func (n *Node) Get(key string) *Node {
	if !n.IsMap() {
		return nil
	}
	return n.values[key]
}

// Path walks nested keys; it returns nil when one is missing.
func (n *Node) Path(keys ...string) *Node {
	for _, k := range keys {
		n = n.Get(k)
		if n == nil {
			return nil
		}
	}
	return n
}

// String returns the scalar at the nested keys, or "".
func (n *Node) String(keys ...string) string {
	c := n.Path(keys...)
	if c == nil || c.isMap {
		return ""
	}
	return c.Scalar
}

// Set stores a child node, appending new keys at the end.
func (n *Node) Set(key string, v *Node) {
	if _, ok := n.values[key]; !ok {
		n.keys = append(n.keys, key)
	}
	n.values[key] = v
}

// SetString stores a scalar child.
func (n *Node) SetString(key, v string) { n.Set(key, &Node{Scalar: v}) }

// Map returns the mapping at key, creating it (or replacing a scalar)
// when needed.
func (n *Node) Map(key string) *Node {
	if c := n.Get(key); c.IsMap() {
		return c
	}
	c := NewMap()
	n.Set(key, c)
	return c
}

// Delete removes key.
func (n *Node) Delete(key string) {
	if _, ok := n.values[key]; !ok {
		return
	}
	delete(n.values, key)
	for i, k := range n.keys {
		if k == key {
			n.keys = append(n.keys[:i], n.keys[i+1:]...)
			break
		}
	}
}

type line struct {
	num    int
	indent int
	text   string
}

// ParseYAML parses the supported subset: block mappings with scalar or
// mapping values, comments, quoted scalars and the empty flow map {}.
func ParseYAML(data []byte) (*Node, error) {
	var lines []line
	for i, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		t := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimLeft(t, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" {
			continue
		}
		if strings.HasPrefix(trimmed, "\t") {
			return nil, fmt.Errorf("line %d: tabs are not allowed for indentation", i+1)
		}
		lines = append(lines, line{num: i + 1, indent: len(t) - len(trimmed), text: trimmed})
	}
	root := NewMap()
	if len(lines) == 0 {
		return root, nil
	}
	next, err := parseBlock(lines, 0, lines[0].indent, root)
	if err != nil {
		return nil, err
	}
	if next < len(lines) {
		return nil, fmt.Errorf("line %d: unexpected indentation", lines[next].num)
	}
	return root, nil
}

func parseBlock(lines []line, i, indent int, into *Node) (int, error) {
	for i < len(lines) {
		l := lines[i]
		if l.indent < indent {
			return i, nil
		}
		if l.indent > indent {
			return i, fmt.Errorf("line %d: unexpected indentation", l.num)
		}
		if strings.HasPrefix(l.text, "- ") || l.text == "-" {
			return i, fmt.Errorf("line %d: YAML lists are not supported", l.num)
		}
		key, rest, err := splitKey(l.text)
		if err != nil {
			return i, fmt.Errorf("line %d: %w", l.num, err)
		}
		i++
		value := stripComment(rest)
		switch {
		case value == "{}":
			into.Set(key, NewMap())
		case value != "":
			s, err := unquote(value)
			if err != nil {
				return i, fmt.Errorf("line %d: %w", l.num, err)
			}
			into.SetString(key, s)
		case i < len(lines) && lines[i].indent > indent:
			child := NewMap()
			if i, err = parseBlock(lines, i, lines[i].indent, child); err != nil {
				return i, err
			}
			into.Set(key, child)
		default:
			into.SetString(key, "")
		}
	}
	return i, nil
}

func splitKey(t string) (string, string, error) {
	if t[0] == '"' || t[0] == '\'' {
		end := strings.IndexByte(t[1:], t[0])
		if end < 0 {
			return "", "", fmt.Errorf("unterminated quoted key")
		}
		key := t[1 : end+1]
		rest := strings.TrimLeft(t[end+2:], " ")
		if !strings.HasPrefix(rest, ":") {
			return "", "", fmt.Errorf("expected ':' after key")
		}
		return key, strings.TrimSpace(rest[1:]), nil
	}
	idx := strings.Index(t, ": ")
	if idx < 0 {
		if strings.HasSuffix(t, ":") {
			return t[:len(t)-1], "", nil
		}
		return "", "", fmt.Errorf("expected 'key: value'")
	}
	return t[:idx], strings.TrimSpace(t[idx+2:]), nil
}

func stripComment(v string) string {
	if v == "" || v[0] == '"' || v[0] == '\'' {
		return v
	}
	if i := strings.Index(v, " #"); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}

func unquote(v string) (string, error) {
	switch v[0] {
	case '"':
		end := strings.LastIndexByte(v, '"')
		if end == 0 {
			return "", fmt.Errorf("unterminated string")
		}
		s, err := strconv.Unquote(v[:end+1])
		if err != nil {
			return "", fmt.Errorf("invalid quoted string: %w", err)
		}
		return s, nil
	case '\'':
		end := strings.LastIndexByte(v, '\'')
		if end == 0 {
			return "", fmt.Errorf("unterminated string")
		}
		return strings.ReplaceAll(v[1:end], "''", "'"), nil
	}
	return v, nil
}

// Encode writes the node as YAML with four-space indentation, as gh does.
func (n *Node) Encode() []byte {
	var b strings.Builder
	n.encode(&b, 0)
	return []byte(b.String())
}

func (n *Node) encode(b *strings.Builder, depth int) {
	pad := strings.Repeat("    ", depth)
	for _, k := range n.keys {
		v := n.values[k]
		b.WriteString(pad + quoteIfNeeded(k) + ":")
		switch {
		case v.isMap && len(v.keys) == 0:
			b.WriteString(" {}\n")
		case v.isMap:
			b.WriteString("\n")
			v.encode(b, depth+1)
		case v.Scalar == "":
			b.WriteString("\n")
		default:
			b.WriteString(" " + quoteIfNeeded(v.Scalar) + "\n")
		}
	}
}

func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\"'\n\t{}[],&*!|>%@`") || strings.TrimSpace(s) != s ||
		s[0] == '-' || s[0] == '?' {
		return strconv.Quote(s)
	}
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "null", "~", "on", "off":
		return strconv.Quote(s)
	}
	return s
}
