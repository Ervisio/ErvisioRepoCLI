// Package release implements `eg release`.
package release

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// New returns the release command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "release <command>",
		Short:   "Manage releases",
		GroupID: "core",
		Example: `$ eg release create v1.0.0 ./dist/*.tgz --notes "First release"
$ eg release list
$ eg release download v1.0.0 -p "*.tgz"`,
	}
	c.AddGroup(cmdutil.Group{ID: "general", Title: "GENERAL COMMANDS"})
	c.AddCommand(newCreate(f), newList(f),
		newDelete(f), newDeleteAsset(f), newDownload(f), newEdit(f), newUpload(f), newView(f))
	return c
}

// Asset is a release asset.
type Asset struct {
	ID                 int64  `json:"id"`
	NodeID             string `json:"node_id"`
	Name               string `json:"name"`
	Label              string `json:"label"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
	DownloadCount      int    `json:"download_count"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// Release is a REST release.
type Release struct {
	ID              int64        `json:"id"`
	NodeID          string       `json:"node_id"`
	TagName         string       `json:"tag_name"`
	TargetCommitish string       `json:"target_commitish"`
	Name            *string      `json:"name"`
	Body            *string      `json:"body"`
	Draft           bool         `json:"draft"`
	Prerelease      bool         `json:"prerelease"`
	CreatedAt       string       `json:"created_at"`
	PublishedAt     *string      `json:"published_at"`
	URL             string       `json:"url"`
	HTMLURL         string       `json:"html_url"`
	UploadURL       string       `json:"upload_url"`
	TarballURL      *string      `json:"tarball_url"`
	ZipballURL      *string      `json:"zipball_url"`
	Author          *shared.User `json:"author"`
	Assets          []Asset      `json:"assets"`
}

// Title is the name, or the tag when unnamed.
func (r *Release) Title() string {
	if r.Name != nil && *r.Name != "" {
		return *r.Name
	}
	return r.TagName
}

func (r *Release) body() string {
	if r.Body == nil {
		return ""
	}
	return *r.Body
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

var releaseJSONFields = []string{
	"apiUrl", "assets", "author", "body", "createdAt", "databaseId", "id", "isDraft", "isPrerelease",
	"name", "publishedAt", "tagName", "tarballUrl", "targetCommitish", "uploadUrl", "url", "zipballUrl",
}

func (r *Release) export() map[string]any {
	assets := []any{}
	for _, a := range r.Assets {
		assets = append(assets, map[string]any{
			"apiUrl": a.URL, "contentType": a.ContentType, "createdAt": a.CreatedAt, "downloadCount": a.DownloadCount,
			"id": a.NodeID, "label": a.Label, "name": a.Name, "size": a.Size, "state": "uploaded",
			"updatedAt": a.UpdatedAt, "url": a.BrowserDownloadURL,
		})
	}
	var author any
	if r.Author != nil {
		author = map[string]any{"id": r.Author.NodeID, "login": r.Author.Login}
	}
	var published any
	if r.PublishedAt != nil {
		published = *r.PublishedAt
	}
	return map[string]any{
		"apiUrl": r.URL, "assets": assets, "author": author, "body": r.body(), "createdAt": r.CreatedAt,
		"databaseId": r.ID, "id": r.NodeID, "isDraft": r.Draft, "isPrerelease": r.Prerelease,
		"name": deref(r.Name), "publishedAt": published, "tagName": r.TagName, "tarballUrl": deref(r.TarballURL),
		"targetCommitish": r.TargetCommitish, "uploadUrl": r.UploadURL, "url": r.HTMLURL, "zipballUrl": deref(r.ZipballURL),
	}
}

// findRelease loads the release of a tag ("" for the latest). Drafts are
// not served by the tags endpoint, so the list is searched as a fallback.
func findRelease(c *api.Client, r ghrepo.Repo, tag string) (*Release, error) {
	var rel Release
	if tag == "" {
		if _, err := c.REST(http.MethodGet, shared.RepoPath(r)+"/releases/latest", nil, &rel); err != nil {
			if api.IsNotFound(err) {
				return nil, fmt.Errorf("release not found")
			}
			return nil, err
		}
		return &rel, nil
	}
	_, err := c.REST(http.MethodGet, shared.RepoPath(r)+"/releases/tags/"+escapePath(tag), nil, &rel)
	if err == nil {
		return &rel, nil
	}
	if !api.IsNotFound(err) {
		return nil, err
	}
	list, _, _, err := shared.Collect[Release](c, shared.RepoPath(r)+"/releases", 1, func(x *Release) bool { return x.TagName == tag })
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("release not found")
	}
	return list[0], nil
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func addRepoFlag(c *cmdutil.Command, p *string) { shared.AddRepoFlag(c, p) }

// humanSize formats a byte count as gh does ("1.23 MiB").
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
