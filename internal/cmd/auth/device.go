package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/build"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// clientID is the built-in OAuth client of eg on ErvisioRepo servers.
const clientID = "Ov23liErvisioEgCli01"

// defaultScopes are the scopes gh requests.
var defaultScopes = []string{"repo", "read:org", "workflow"}

type deviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenAnswer struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
	Interval    int    `json:"interval"`
}

func postForm(base, p string, form url.Values, out any) error {
	req, err := http.NewRequest(http.MethodPost, base+p, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Ervisio CLI "+build.Version)
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return fmt.Errorf("error connecting to %s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode == http.StatusNotFound {
		return cmdutil.Unsupported("Browser-based login (OAuth device flow)", "POST "+p)
	}
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, req.URL)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response from %s: %w", req.URL, err)
	}
	return nil
}

// deviceFlow runs the OAuth device authorization grant and returns the
// access token. eg starts no process other than git, so the user opens
// the verification page.
func deviceFlow(f *cmdutil.Factory, scheme, host string, extraScopes []string) (string, error) {
	ios := f.IO
	cs := ios.ColorScheme()
	base := scheme + "://" + host
	scopes := append(append([]string(nil), defaultScopes...), extraScopes...)
	form := url.Values{"client_id": {clientID}, "scope": {strings.Join(scopes, " ")}}
	if name, err := os.Hostname(); err == nil && name != "" {
		form.Set("device_name", name)
	}
	var dc deviceCode
	if err := postForm(base, "/login/device/code", form, &dc); err != nil {
		return "", err
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return "", errors.New("the server did not return a device code")
	}
	if dc.VerificationURI == "" {
		dc.VerificationURI = base + "/login/device"
	}
	fmt.Fprintf(ios.ErrOut, "%s First copy your one-time code: %s\n", cs.Yellow("!"), cs.Bold(dc.UserCode))
	if ios.CanPrompt() {
		fmt.Fprintf(ios.ErrOut, "Press Enter to show the page to open in your browser... ")
		if _, err := ios.ReadLine(); err != nil {
			return "", cmdutil.ErrCancel
		}
	}
	fmt.Fprintf(ios.ErrOut, "Open this URL in your browser: %s\n", cs.Bold(dc.VerificationURI+"?user_code="+url.QueryEscape(dc.UserCode)))

	interval := time.Duration(max(dc.Interval, 5)) * time.Second
	expires := time.Now().Add(time.Duration(max(dc.ExpiresIn, 60)) * time.Second)
	poll := url.Values{"client_id": {clientID}, "device_code": {dc.DeviceCode},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}}
	for {
		time.Sleep(interval)
		if time.Now().After(expires) {
			return "", errors.New("the device code expired; run `eg auth login` again")
		}
		var ta tokenAnswer
		if err := postForm(base, "/login/oauth/access_token", poll, &ta); err != nil {
			return "", err
		}
		switch ta.Error {
		case "":
			if ta.AccessToken == "" {
				return "", errors.New("the server returned no access token")
			}
			fmt.Fprintf(ios.ErrOut, "%s Authentication complete.\n", cs.SuccessIcon())
			return ta.AccessToken, nil
		case "authorization_pending":
		case "slow_down":
			if ta.Interval > 0 {
				interval = time.Duration(ta.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return "", errors.New("the device code expired; run `eg auth login` again")
		case "access_denied":
			return "", errors.New("authorization was denied")
		default:
			msg := ta.Error
			if ta.ErrorDesc != "" {
				msg += ": " + ta.ErrorDesc
			}
			return "", fmt.Errorf("device authorization failed: %s", msg)
		}
	}
}
