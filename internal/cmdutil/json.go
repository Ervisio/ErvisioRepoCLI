package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
	"github.com/Ervisio/ErvisioRepoCLI/internal/jsonx"
)

// Exporter holds the --json, --jq and --template flags.
type Exporter struct {
	fields   string
	jq       string
	template string
	allowed  []string
}

// AddJSONFlags registers --json, -q/--jq and -t/--template on cmd.
func AddJSONFlags(cmd *Command, e *Exporter, fields []string) {
	e.allowed = fields
	cmd.JSONFields = fields
	cmd.Flags.StringVarP(&e.fields, "json", "", "", "Output JSON with the specified `fields`")
	cmd.Flags.StringVarP(&e.jq, "jq", "q", "", "Filter JSON output using a jq `expression` (simple paths only)")
	cmd.Flags.StringVarP(&e.template, "template", "t", "", "Format JSON output using a Go template; not supported by eg yet")
}

// Enabled reports whether JSON output was requested.
func (e *Exporter) Enabled() bool { return e.fields != "" }

// Validate checks the flag combination and the requested fields.
func (e *Exporter) Validate() error {
	if e.template != "" {
		return FlagErrorf("--template is not supported by eg yet; use --jq or --json")
	}
	if e.fields == "" {
		if e.jq != "" {
			return FlagErrorf("cannot use `--jq` without specifying `--json`")
		}
		return nil
	}
	for _, f := range e.Fields() {
		ok := false
		for _, a := range e.allowed {
			if a == f {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("Unknown JSON field: %q\n%s", f, jsonFieldsError(e.allowed).Error()) //nolint:staticcheck // gh wording
		}
	}
	return nil
}

// Fields lists the requested fields.
func (e *Exporter) Fields() []string {
	var out []string
	for _, f := range strings.Split(e.fields, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// Pick keeps the requested fields of an object.
func (e *Exporter) Pick(all map[string]any) map[string]any {
	out := map[string]any{}
	for _, f := range e.Fields() {
		out[f] = all[f]
	}
	return out
}

// Write prints data (already reduced with Pick).
func (e *Exporter) Write(ios *iostreams.IOStreams, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	if e.jq != "" {
		return jsonx.Query(ios.Out, bytes.NewReader(b), e.jq)
	}
	return jsonx.Write(ios.Out, bytes.NewReader(b), ios.ColorEnabled())
}
