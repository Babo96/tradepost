// Package update checks GitHub releases for a newer TradePost companion and
// installs the downloaded binary and addon sources in place.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAPIURL is the latest-release endpoint for the public repository.
	DefaultAPIURL = "https://api.github.com/repos/Babo96/tradepost/releases/latest"
	// DefaultTimeout bounds a release check.
	DefaultTimeout = 20 * time.Second
	userAgentName  = "tradepost-companion"
)

// Asset is one downloadable release file.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Digest      string `json:"digest"`
	Size        int64  `json:"size"`
}

// Info is the update state shared with the dashboard and CLI.
type Info struct {
	Current         string    `json:"current"`
	Latest          string    `json:"latest,omitempty"`
	Available       bool      `json:"available"`
	Notes           string    `json:"notes,omitempty"`
	HTMLURL         string    `json:"html_url,omitempty"`
	AssetName       string    `json:"asset_name,omitempty"`
	AssetURL        string    `json:"asset_url,omitempty"`
	Digest          string    `json:"digest,omitempty"`
	Size            int64     `json:"size,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
	Error           string    `json:"error,omitempty"`
	Applying        bool      `json:"applying"`
	Applied         bool      `json:"applied"`
	RestartRequired bool      `json:"restart_required"`
	AddonUpdated    bool      `json:"addon_updated"`
	Message         string    `json:"message,omitempty"`
}

// Client checks the public GitHub releases API.
type Client struct {
	Version string
	APIURL  string
	HTTP    *http.Client
}

// New returns a client for the given current companion version.
func New(version string) *Client {
	return &Client{
		Version: version,
		APIURL:  DefaultAPIURL,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
	}
}

type releaseResponse struct {
	TagName string  `json:"tag_name"`
	Body    string  `json:"body"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Check fetches the latest release and fills in the update info. The returned
// error is also recorded in Info.Error so callers can keep showing status.
func (c *Client) Check(ctx context.Context) (Info, error) {
	info := Info{Current: c.Version, CheckedAt: time.Now().UTC()}
	if strings.TrimSpace(c.Version) == "" {
		info.Current = "dev"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL, nil)
	if err != nil {
		info.Error = err.Error()
		return info, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgentName+"/"+c.Version)

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("github: %s", resp.Status)
		info.Error = err.Error()
		return info, err
	}

	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		info.Error = err.Error()
		return info, err
	}
	info.Latest = NormalizeVersion(release.TagName)
	info.Notes = release.Body
	info.HTMLURL = release.HTMLURL
	info.Available = IsNewer(info.Latest, c.Version)

	asset, err := SelectAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		// A newer release without a matching asset is still interesting, but it
		// cannot be installed on this machine.
		info.Error = err.Error()
	} else {
		info.AssetName = asset.Name
		info.AssetURL = asset.DownloadURL
		info.Digest = asset.Digest
		info.Size = asset.Size
	}
	return info, nil
}

// SelectAsset picks the release asset for the given platform.
func SelectAsset(assets []Asset, goos, goarch string) (Asset, error) {
	var suffix string
	switch {
	case goos == "linux" && goarch == "amd64":
		suffix = "linux-amd64.tar.gz"
	case goos == "windows" && goarch == "amd64":
		suffix = "windows-amd64.zip"
	default:
		return Asset{}, fmt.Errorf("no companion build for %s/%s", goos, goarch)
	}
	for _, asset := range assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), suffix) {
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("release has no %s asset for %s/%s", suffix, goos, goarch)
}

// NormalizeVersion strips a leading v and any -dirty/+build suffix.
func NormalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	if index := strings.IndexAny(version, "-+"); index >= 0 {
		version = version[:index]
	}
	return strings.TrimSpace(version)
}

// CompareVersions returns -1, 0, or 1 comparing a and b as major.minor.patch.
// Versions that cannot be parsed compare as equal unless the raw strings
// differ, in which case a string comparison is used.
func CompareVersions(a, b string) int {
	av, aok := parseVersion(a)
	bv, bok := parseVersion(b)
	if !aok || !bok {
		return strings.Compare(NormalizeVersion(a), NormalizeVersion(b))
	}
	for i := range av {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	return 0
}

// IsNewer reports whether latest is strictly newer than current.
func IsNewer(latest, current string) bool {
	return CompareVersions(latest, current) > 0
}

func parseVersion(version string) ([3]int, bool) {
	var out [3]int
	version = NormalizeVersion(version)
	if version == "" {
		return out, false
	}
	parts := strings.Split(version, ".")
	if len(parts) > 3 {
		return out, false
	}
	for i, part := range parts {
		if part == "" {
			return out, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// ErrNoUpdate is returned when there is nothing to install.
var ErrNoUpdate = errors.New("already up to date")
