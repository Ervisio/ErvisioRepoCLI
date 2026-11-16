// Package api implements `eg api`.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	eapi "github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/jsonx"
)

type options struct {
	method    string
	rawFields []string
	fields    []string
	headers   []string
	previews  []string
	paginate  bool
	slurp     bool
	input     string
	include   bool
	silent    bool
	hostname  string
	jq        string
	template  string
	verbose   bool
}

// New returns the api command.
func New(f *cmdutil.Factory) *cmdutil.Command {
	o := &options{}
	c := &cmdutil.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated ErvisioRepo API request",
		Long: `Makes an authenticated HTTP request to the GitHub-compatible API of an
ErvisioRepo server and prints the response.

The endpoint argument should either be a path of an API v3 endpoint, or
"graphql" to access the GraphQL API.

Placeholder values "{owner}", "{repo}", and "{branch}" in the endpoint
argument will get replaced with values from the repository of the current
directory or the repository specified in the EG_REPO environment variable.
Note that in some shells, for example PowerShell, you may need to enclose
any value that contains "{...}" in quotes to prevent the shell from
applying special meaning to curly braces.

The default HTTP request method is "GET" normally and "POST" if any
parameters were added. Override the method with --method.

Pass one or more -f/--raw-field values in "key=value" format to add static
string parameters to the request payload. To add non-string or placeholder-
determined values, see -F/--field below. Note that adding request parameters
will automatically switch the request method to "POST". To send the
parameters as a GET query string instead, use --method GET.

The -F/--field flag has magic type conversion based on the format of the
value:

- literal values "true", "false", "null", and integer numbers get converted
  to appropriate JSON types;
- placeholder values "{owner}", "{repo}", and "{branch}" get populated with
  values from the repository of the current directory;
- if the value starts with "@", the rest of the value is interpreted as a
  filename to read the value from. Pass "-" to read from standard input.

For GraphQL requests, all fields other than "query" and "operationName" are
interpreted as GraphQL variables.

To pass nested parameters in the request payload, use "key[subkey]=value"
syntax when declaring fields. To pass nested values as arrays, declare
multiple fields with the syntax "key[]=value1", "key[]=value2". To pass an
empty array, use "key[]" without a value.

To pass pre-constructed JSON or payloads in other formats, a request body
may be read from file specified by --input. Use "-" to read from standard
input. When passing the request body this way, any parameters specified
via field flags are added to the query string of the endpoint URL.

In --paginate mode, all pages of results will sequentially be requested
until there are no more pages of results. For GraphQL requests, this
requires that the original query accepts an $endCursor: String variable
and that it fetches the pageInfo{ hasNextPage, endCursor } set of fields
from a collection.

The --jq option accepts simple paths only ('.name', '.[].name',
'.items | .[0]'); --template is not supported yet.`,
		Example: `# list releases in the current repository
$ eg api repos/{owner}/{repo}/releases

# post an issue comment
$ eg api repos/{owner}/{repo}/issues/123/comments -f body='Hi from CLI'

# add parameters to a GET request
$ eg api -X GET users/{owner}/repos -F per_page=100

# opt into GitHub API previews
$ eg api --preview baptiste,nebula ...

# print only specific fields from the response
$ eg api repos/{owner}/{repo}/issues --jq '.[].title'

# use a template for the output (not supported yet)

# list releases with GraphQL
$ eg api graphql -F owner='{owner}' -F name='{repo}' -f query='
  query($name: String!, $owner: String!) {
    repository(owner: $owner, name: $name) {
      releases(last: 3) {
        nodes { tagName }
      }
    }
  }
'

# list all repositories for a user
$ eg api graphql --paginate -f query='
  query($endCursor: String) {
    viewer {
      repositories(first: 100, after: $endCursor) {
        nodes { nameWithOwner }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
'`,
		Flags: cmdutil.NewFlagSet("api"),
		Args:  cmdutil.ExactArgs(1, "must pass a single endpoint"),
	}
	fl := c.Flags
	fl.StringVarP(&o.hostname, "hostname", "", "", "The ErvisioRepo `hostname` for the request")
	fl.StringVarP(&o.method, "method", "X", "GET", "The HTTP method for the request")
	fl.StringArrayVarP(&o.fields, "field", "F", "Add a typed parameter in `key=value` format")
	fl.StringArrayVarP(&o.rawFields, "raw-field", "f", "Add a string parameter in `key=value` format")
	fl.StringArrayVarP(&o.headers, "header", "H", "Add a HTTP request header in `key:value` format")
	fl.StringArrayVarP(&o.previews, "preview", "p", "Opt into GitHub API previews (`names`)")
	fl.BoolVarP(&o.include, "include", "i", false, "Include HTTP response status line and headers in the output")
	fl.BoolVarP(&o.paginate, "paginate", "", false, "Make additional HTTP requests to fetch all pages of results")
	fl.BoolVarP(&o.slurp, "slurp", "", false, "Use with \"--paginate\" to return an array of all pages of either JSON arrays or objects")
	fl.StringVarP(&o.input, "input", "", "", "The `file` to use as body for the HTTP request (use \"-\" to read from standard input)")
	fl.BoolVarP(&o.silent, "silent", "", false, "Do not print the response body")
	fl.StringVarP(&o.jq, "jq", "q", "", "Query to select values from the response using jq syntax (simple paths only)")
	fl.StringVarP(&o.template, "template", "t", "", "Format JSON output using a Go template; not supported by eg yet")
	fl.BoolVarP(&o.verbose, "verbose", "", false, "Include full HTTP request and response in the output")
	c.Run = func(c *cmdutil.Command, args []string) error { return run(f, c, o, args[0]) }
	return c
}

func run(f *cmdutil.Factory, c *cmdutil.Command, o *options, endpoint string) error {
	if o.template != "" {
		return cmdutil.FlagErrorf("--template is not supported by eg yet; use --jq")
	}
	if o.slurp && !o.paginate {
		return cmdutil.FlagErrorf("`--paginate` required when passing `--slurp`")
	}
	if o.slurp && o.jq != "" {
		return cmdutil.FlagErrorf("the `--slurp` option is not supported with `--jq`")
	}
	if o.silent && o.jq != "" {
		return cmdutil.FlagErrorf("only one of `--silent` or `--jq` may be used")
	}
	methodSet := c.Flags.Changed("method")
	o.method = strings.ToUpper(o.method)

	// Placeholders.
	var repo *ghrepo.Repo
	needsRepo := hasPlaceholder(endpoint) || anyPlaceholder(o.fields)
	if needsRepo {
		r, err := f.BaseRepo("")
		if err != nil {
			return err
		}
		repo = &r
	}
	branch := ""
	if strings.Contains(endpoint, "{branch}") || anyBranchPlaceholder(o.fields) {
		b, err := f.Git("").CurrentBranch()
		if err != nil {
			return err
		}
		branch = b
	}
	fill := func(s string) string {
		if repo != nil {
			s = strings.ReplaceAll(s, "{owner}", repo.Owner)
			s = strings.ReplaceAll(s, "{repo}", repo.Name)
		}
		if branch != "" {
			s = strings.ReplaceAll(s, "{branch}", branch)
		}
		return s
	}
	endpoint = fill(endpoint)

	host := ""
	if o.hostname != "" {
		h, _, err := config.ParseHostInput(o.hostname)
		if err != nil {
			return err
		}
		host = h
	} else if repo != nil {
		host = repo.Host
	} else if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		host = u.Host
	} else {
		h, err := f.DefaultHost()
		if err != nil {
			return err
		}
		host = h
	}
	client, err := f.Client(host)
	if err != nil {
		return err
	}
	if o.verbose {
		client.Debug = f.IO.ErrOut
	}

	params, err := buildParams(f, o, fill)
	if err != nil {
		return err
	}
	isGraphQL := endpoint == "graphql"
	if (len(params) > 0 || o.input != "") && !methodSet {
		o.method = http.MethodPost
	}

	if o.paginate && !isGraphQL && o.method != http.MethodGet {
		return cmdutil.FlagErrorf("the `--paginate` option is not supported for non-GET requests")
	}

	var reqURL string
	var body []byte
	if isGraphQL {
		reqURL = client.GraphQLURL()
		o.method = http.MethodPost
	} else {
		reqURL = client.RESTURL(endpoint)
	}
	if o.input != "" {
		if body, err = readInput(f, o.input); err != nil {
			return err
		}
		if len(params) > 0 {
			reqURL = addQuery(reqURL, params)
		}
	} else if isGraphQL {
		gq := map[string]any{}
		vars := map[string]any{}
		for k, v := range params {
			if k == "query" || k == "operationName" {
				gq[k] = v
			} else {
				vars[k] = v
			}
		}
		if len(vars) > 0 {
			gq["variables"] = vars
		}
		if body, err = json.Marshal(gq); err != nil {
			return fmt.Errorf("encode GraphQL request: %w", err)
		}
	} else if len(params) > 0 {
		if o.method == http.MethodGet || o.method == http.MethodHead {
			reqURL = addQuery(reqURL, params)
		} else if body, err = json.Marshal(params); err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
	}

	header := http.Header{}
	for _, h := range o.headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return cmdutil.FlagErrorf("header %q requires a value separated by ':'", h)
		}
		header.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	if len(o.previews) > 0 && header.Get("Accept") == "" {
		var accepts []string
		for _, p := range o.previews {
			for _, n := range strings.Split(p, ",") {
				accepts = append(accepts, "application/vnd.github."+strings.TrimSpace(n)+"-preview+json")
			}
		}
		header.Set("Accept", strings.Join(accepts, ", "))
	}

	var pages []json.RawMessage
	allArrays := true
	hasNext := true
	for page := 0; hasNext; page++ {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		resp, err := client.Request(o.method, reqURL, rdr, header)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck,gosec // fully read
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		if o.include {
			writeHeaders(f, resp)
		}
		isJSON := strings.Contains(resp.Header.Get("Content-Type"), "json")
		if resp.StatusCode >= 400 {
			if !o.silent && len(data) > 0 {
				if err := output(f, o, data, isJSON, false); err != nil {
					return err
				}
			}
			fmt.Fprintf(f.IO.ErrOut, "eg: %s (HTTP %d)\n", errorMessage(data, resp.StatusCode), resp.StatusCode)
			if resp.StatusCode == http.StatusUnauthorized {
				fmt.Fprintf(f.IO.ErrOut, "Try authenticating with:  eg auth login -h %s\n", host)
			}
			return cmdutil.ErrSilent
		}
		if isGraphQL {
			if msg := graphqlErrors(data); msg != "" {
				if !o.silent {
					_ = output(f, o, data, isJSON, false)
				}
				fmt.Fprintf(f.IO.ErrOut, "eg: %s\n", msg)
				return cmdutil.ErrSilent
			}
		}

		hasNext = false
		if o.paginate {
			if isGraphQL {
				if cursor, ok := graphqlNextCursor(data); ok {
					var gq map[string]any
					if json.Unmarshal(body, &gq) == nil {
						vars, _ := gq["variables"].(map[string]any)
						if vars == nil {
							vars = map[string]any{}
						}
						vars["endCursor"] = cursor
						gq["variables"] = vars
						if nb, err := json.Marshal(gq); err == nil {
							body, hasNext = nb, true
						}
					}
				}
			} else if next := eapi.NextPage(resp.Header.Get("Link")); next != "" {
				reqURL, hasNext = next, true
			}
		}
		if !o.paginate || !isJSON {
			if !o.silent {
				if err := output(f, o, data, isJSON, false); err != nil {
					return err
				}
			}
			continue
		}
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 || trimmed[0] != '[' {
			allArrays = false
		}
		pages = append(pages, json.RawMessage(trimmed))
	}
	if len(pages) == 0 || o.silent {
		return nil
	}
	return output(f, o, mergePages(pages, allArrays && !o.slurp, o.slurp), true, true)
}

func mergePages(pages []json.RawMessage, mergeArrays, slurp bool) []byte {
	switch {
	case slurp:
		b, _ := json.Marshal(pages)
		return b
	case mergeArrays:
		var all []json.RawMessage
		for _, p := range pages {
			var items []json.RawMessage
			if json.Unmarshal(p, &items) == nil {
				all = append(all, items...)
			}
		}
		if all == nil {
			all = []json.RawMessage{}
		}
		b, _ := json.Marshal(all)
		return b
	}
	var b bytes.Buffer
	for i, p := range pages {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(p)
	}
	return b.Bytes()
}

func output(f *cmdutil.Factory, o *options, data []byte, isJSON, reencoded bool) error {
	ios := f.IO
	if o.jq != "" && isJSON {
		return jsonx.Query(ios.Out, bytes.NewReader(data), o.jq)
	}
	if isJSON && ios.IsStdoutTTY() {
		if err := jsonx.Write(ios.Out, bytes.NewReader(data), ios.ColorEnabled()); err == nil {
			return nil
		}
	}
	if _, err := ios.Out.Write(data); err != nil {
		return err
	}
	if reencoded && len(data) > 0 && data[len(data)-1] != '\n' {
		_, err := io.WriteString(ios.Out, "\n")
		return err
	}
	return nil
}

func writeHeaders(f *cmdutil.Factory, resp *http.Response) {
	cs := f.IO.ColorScheme()
	fmt.Fprintf(f.IO.Out, "%s %s\n", resp.Proto, resp.Status)
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(f.IO.Out, "%s: %s\n", cs.Blue(k), strings.Join(resp.Header[k], ", "))
	}
	fmt.Fprintln(f.IO.Out)
}

func errorMessage(data []byte, status int) string {
	var e struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(data, &e) == nil {
		msgs := []string{}
		if e.Message != "" {
			msgs = append(msgs, e.Message)
		}
		for _, x := range e.Errors {
			if x.Message != "" {
				msgs = append(msgs, x.Message)
			}
		}
		if len(msgs) > 0 {
			return strings.Join(msgs, "\n")
		}
	}
	return http.StatusText(status)
}

func graphqlErrors(data []byte) string {
	var r struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(data, &r) != nil || len(r.Errors) == 0 {
		return ""
	}
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = e.Message
	}
	return strings.Join(msgs, "\n")
}

// graphqlNextCursor finds the first pageInfo with hasNextPage.
func graphqlNextCursor(data []byte) (string, bool) {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return "", false
	}
	var walk func(any) (string, bool, bool)
	walk = func(x any) (string, bool, bool) {
		switch t := x.(type) {
		case map[string]any:
			if pi, ok := t["pageInfo"].(map[string]any); ok {
				next, _ := pi["hasNextPage"].(bool)
				cur, _ := pi["endCursor"].(string)
				return cur, next, true
			}
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if c, n, found := walk(t[k]); found {
					return c, n, true
				}
			}
		case []any:
			for _, e := range t {
				if c, n, found := walk(e); found {
					return c, n, true
				}
			}
		}
		return "", false, false
	}
	cur, next, _ := walk(v)
	return cur, next && cur != ""
}

func readInput(f *cmdutil.Factory, name string) ([]byte, error) {
	if name == "-" {
		b, err := io.ReadAll(f.IO.In)
		if err != nil {
			return nil, fmt.Errorf("read standard input: %w", err)
		}
		return b, nil
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read --input file: %w", err)
	}
	return b, nil
}

func hasPlaceholder(s string) bool {
	return strings.Contains(s, "{owner}") || strings.Contains(s, "{repo}")
}

func anyPlaceholder(fields []string) bool {
	for _, f := range fields {
		if _, v, ok := strings.Cut(f, "="); ok && hasPlaceholder(v) {
			return true
		}
	}
	return false
}

func anyBranchPlaceholder(fields []string) bool {
	for _, f := range fields {
		if _, v, ok := strings.Cut(f, "="); ok && strings.Contains(v, "{branch}") {
			return true
		}
	}
	return false
}

// buildParams turns -f/-F into a (possibly nested) map.
func buildParams(f *cmdutil.Factory, o *options, fill func(string) string) (map[string]any, error) {
	params := map[string]any{}
	add := func(raw string, typed bool) error {
		key, value, hasValue := strings.Cut(raw, "=")
		if !hasValue && !strings.HasSuffix(key, "[]") {
			return cmdutil.FlagErrorf("field %q requires a value separated by an '=' sign", raw)
		}
		var v any = value
		if typed {
			var err error
			if v, err = magicValue(f, fill, value); err != nil {
				return err
			}
		}
		if !hasValue {
			v = nil
		}
		return setNested(params, key, v, hasValue)
	}
	for _, r := range o.rawFields {
		if err := add(r, false); err != nil {
			return nil, err
		}
	}
	for _, r := range o.fields {
		if err := add(r, true); err != nil {
			return nil, err
		}
	}
	return params, nil
}

func magicValue(f *cmdutil.Factory, fill func(string) string, v string) (any, error) {
	switch {
	case v == "true":
		return true, nil
	case v == "false":
		return false, nil
	case v == "null":
		return nil, nil
	case strings.HasPrefix(v, "@"):
		b, err := readInput(f, v[1:])
		if err != nil {
			return nil, err
		}
		return string(b), nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}
	return fill(v), nil
}

// setNested stores v at key, which may be "a[b][]".
func setNested(m map[string]any, key string, v any, hasValue bool) error {
	parts, err := splitKey(key)
	if err != nil {
		return err
	}
	cur := m
	for i, p := range parts {
		last := i == len(parts)-1
		nextIsArray := !last && parts[i+1] == ""
		switch {
		case last:
			cur[p] = v
		case nextIsArray && i+1 == len(parts)-1:
			arr, _ := cur[p].([]any)
			if arr == nil {
				arr = []any{}
			}
			if hasValue {
				arr = append(arr, v)
			}
			cur[p] = arr
			return nil
		case nextIsArray:
			return cmdutil.FlagErrorf("nested arrays of objects are not supported: %q", key)
		default:
			child, ok := cur[p].(map[string]any)
			if !ok {
				child = map[string]any{}
				cur[p] = child
			}
			cur = child
		}
	}
	return nil
}

func splitKey(key string) ([]string, error) {
	i := strings.IndexByte(key, '[')
	if i < 0 {
		return []string{key}, nil
	}
	parts := []string{key[:i]}
	rest := key[i:]
	for rest != "" {
		if rest[0] != '[' {
			return nil, cmdutil.FlagErrorf("invalid key: %q", key)
		}
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return nil, cmdutil.FlagErrorf("invalid key: %q", key)
		}
		parts = append(parts, rest[1:end])
		rest = rest[end+1:]
	}
	return parts, nil
}

// addQuery appends params to the query string; nested values are
// flattened with the bracket syntax.
func addQuery(raw string, params map[string]any) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	var add func(prefix string, v any)
	add = func(prefix string, v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				add(prefix+"["+k+"]", x)
			}
		case []any:
			for _, x := range t {
				add(prefix+"[]", x)
			}
		case nil:
			q.Add(prefix, "")
		default:
			q.Add(prefix, fmt.Sprint(t))
		}
	}
	for k, v := range params {
		add(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
