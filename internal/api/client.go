// Package api is the client of the GitHub-compatible API of an
// ErvisioRepo server: REST under /api/v3 and GraphQL at /api/graphql.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/build"
)

// Client talks to one host.
type Client struct {
	Host   string
	Scheme string
	Token  string
	HTTP   *http.Client
	// Debug receives a trace of every request when non-nil.
	Debug io.Writer
}

// New returns a client for host. scheme is "https" unless the host is a
// plain-http local server.
func New(host, scheme, token string) *Client {
	if scheme == "" {
		scheme = "https"
	}
	c := &Client{Host: host, Scheme: scheme, Token: token, HTTP: &http.Client{Timeout: 5 * time.Minute}}
	if v := os.Getenv("EG_DEBUG"); v != "" && v != "0" && v != "false" {
		c.Debug = os.Stderr
	}
	return c
}

// BaseURL is scheme://host.
func (c *Client) BaseURL() string { return c.Scheme + "://" + c.Host }

// RESTURL resolves a path relative to /api/v3; absolute URLs are kept.
func (c *Client) RESTURL(p string) string {
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	return c.BaseURL() + "/api/v3/" + strings.TrimPrefix(p, "/")
}

// GraphQLURL is the GraphQL endpoint.
func (c *Client) GraphQLURL() string { return c.BaseURL() + "/api/graphql" }

// Request sends a request with the token and default headers; the caller
// owns the response body.
func (c *Client) Request(method, rawURL string, body io.Reader, header http.Header) (*http.Response, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	req.Header.Set("User-Agent", "Ervisio CLI "+build.Version)
	if c.Token != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "token "+c.Token)
	}
	if c.Debug != nil {
		fmt.Fprintf(c.Debug, "* Request to %s\n> %s %s\n", rawURL, method, req.URL.RequestURI())
	}
	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s: %w", c.Host, err)
	}
	if c.Debug != nil {
		fmt.Fprintf(c.Debug, "< HTTP %s (%s)\n", resp.Status, time.Since(start).Round(time.Millisecond))
	}
	return resp, nil
}

// REST sends a JSON request and decodes a JSON answer into out (when not
// nil). It returns the URL of the next page from the Link header.
func (c *Client) REST(method, p string, in, out any) (string, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return "", fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	resp, err := c.Request(method, c.RESTURL(p), body, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 300 {
		return "", HandleHTTPError(resp)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return "", fmt.Errorf("decode response from %s: %w", resp.Request.URL, err)
		}
	}
	return NextPage(resp.Header.Get("Link")), nil
}

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

// NextPage returns the rel="next" URL of a Link header, or "".
func NextPage(link string) string {
	for _, m := range linkRE.FindAllStringSubmatch(link, -1) {
		if m[2] == "next" {
			return m[1]
		}
	}
	return ""
}

// GraphQLError is an error of a GraphQL answer.
type GraphQLError struct {
	Errors []struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Path    []any  `json:"path"`
	}
}

func (e *GraphQLError) Error() string {
	msgs := make([]string, 0, len(e.Errors))
	for _, x := range e.Errors {
		msgs = append(msgs, x.Message)
	}
	return "GraphQL: " + strings.Join(msgs, ", ")
}

// GraphQL runs a query and decodes its data into out.
func (c *Client) GraphQL(query string, vars map[string]any, out any) error {
	b, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("encode query: %w", err)
	}
	resp, err := c.Request(http.MethodPost, c.GraphQLURL(), bytes.NewReader(b), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 300 {
		return HandleHTTPError(resp)
	}
	var res struct {
		Data   json.RawMessage `json:"data"`
		Errors json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("decode GraphQL response: %w", err)
	}
	if len(res.Errors) > 0 && string(res.Errors) != "null" {
		var ge GraphQLError
		if err := json.Unmarshal(res.Errors, &ge.Errors); err == nil && len(ge.Errors) > 0 {
			return &ge
		}
	}
	if out != nil {
		if err := json.Unmarshal(res.Data, out); err != nil {
			return fmt.Errorf("decode GraphQL data: %w", err)
		}
	}
	return nil
}

// HTTPError is a non-2xx answer of the REST API.
type HTTPError struct {
	StatusCode int
	Message    string
	RequestURL *url.URL
	Errors     []FieldError
	// Scopes is the X-OAuth-Scopes header of the answer.
	Scopes string
}

// FieldError is one entry of the "errors" list of a 422.
type FieldError struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func (e *HTTPError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	var details []string
	for _, fe := range e.Errors {
		if fe.Message != "" {
			details = append(details, fe.Message)
		} else if fe.Field != "" {
			details = append(details, fmt.Sprintf("%s.%s is %s", fe.Resource, fe.Field, fe.Code))
		}
	}
	if len(details) > 0 {
		msg += " (" + strings.Join(details, "; ") + ")"
	}
	if e.RequestURL != nil {
		return fmt.Sprintf("HTTP %d: %s (%s)", e.StatusCode, msg, e.RequestURL)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, msg)
}

// HandleHTTPError builds an *HTTPError from a response, reading its body.
func HandleHTTPError(resp *http.Response) error {
	e := &HTTPError{StatusCode: resp.StatusCode, RequestURL: resp.Request.URL, Scopes: resp.Header.Get("X-Oauth-Scopes")}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		e.Message = err.Error()
		return e
	}
	var parsed struct {
		Message string       `json:"message"`
		Errors  []FieldError `json:"errors"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		e.Message, e.Errors = parsed.Message, parsed.Errors
	}
	return e
}

// IsNotFound reports a 404.
func IsNotFound(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.StatusCode == http.StatusNotFound
}

// IsStatus reports an HTTP error with the given status.
func IsStatus(err error, status int) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.StatusCode == status
}

// Upload sends a raw body of a known size (release assets need a
// Content-Length) and decodes the JSON answer into out.
func (c *Client) Upload(rawURL string, body io.Reader, size int64, contentType string, out any) error {
	req, err := http.NewRequest(http.MethodPost, rawURL, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Ervisio CLI "+build.Version)
	if c.Token != "" {
		req.Header.Set("Authorization", "token "+c.Token)
	}
	if c.Debug != nil {
		fmt.Fprintf(c.Debug, "* Request to %s\n> POST %s (%d bytes)\n", rawURL, req.URL.RequestURI(), size)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("error connecting to %s: %w", c.Host, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 300 {
		return HandleHTTPError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response from %s: %w", rawURL, err)
		}
	}
	return nil
}
