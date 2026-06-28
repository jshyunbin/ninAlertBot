// Package updater provides a manual, apt-style self-update: check the latest
// GitHub release and, if newer, download the matching asset, verify its
// checksum, and replace the running binary in place.
package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

// Defaults identifying the upstream repository.
const (
	DefaultOwner   = "jshyunbin"
	DefaultRepo    = "ninAlertBot"
	DefaultAPIBase = "https://api.github.com"
)

// Updater checks for and applies releases. Fields are exported so they can be
// overridden in tests; New fills in production defaults.
type Updater struct {
	Owner   string
	Repo    string
	Current string
	APIBase string
	HTTP    *http.Client
	// Apply replaces the running binary with the bytes read from r. Defaults to
	// selfupdate.Apply; overridden in tests.
	Apply func(r io.Reader) error
	// GOOS/GOARCH select the release asset; default to the build's runtime.
	GOOS   string
	GOARCH string
}

// New returns an Updater configured for the live GitHub repo.
func New(current string) *Updater {
	return &Updater{
		Owner:   DefaultOwner,
		Repo:    DefaultRepo,
		Current: current,
		APIBase: DefaultAPIBase,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		Apply:   func(r io.Reader) error { return selfupdate.Apply(r, selfupdate.Options{}) },
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
	}
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest fetches the latest release metadata.
func (u *Updater) Latest(ctx context.Context) (*release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", strings.TrimRight(u.APIBase, "/"), u.Owner, u.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// CheckResult summarizes a check without applying anything.
type CheckResult struct {
	Current   string
	Latest    string
	HasUpdate bool
	URL       string
}

// Check reports whether a newer release is available.
func (u *Updater) Check(ctx context.Context) (*CheckResult, error) {
	rel, err := u.Latest(ctx)
	if err != nil {
		return nil, err
	}
	return &CheckResult{
		Current:   u.Current,
		Latest:    rel.TagName,
		HasUpdate: isNewer(u.Current, rel.TagName),
		URL:       rel.HTMLURL,
	}, nil
}

// Update checks for a newer release and, if found, downloads, verifies, and
// installs it. Progress is written to out. It returns the result of the check;
// when HasUpdate is false nothing is changed.
func (u *Updater) Update(ctx context.Context, out io.Writer) (*CheckResult, error) {
	rel, err := u.Latest(ctx)
	if err != nil {
		return nil, err
	}
	res := &CheckResult{
		Current:   u.Current,
		Latest:    rel.TagName,
		HasUpdate: isNewer(u.Current, rel.TagName),
		URL:       rel.HTMLURL,
	}
	if !res.HasUpdate {
		return res, nil
	}

	zipName := AssetName(rel.TagName, u.GOOS, u.GOARCH)
	zipURL, sumsURL := "", ""
	for _, a := range rel.Assets {
		switch a.Name {
		case zipName:
			zipURL = a.URL
		case "SHA256SUMS.txt":
			sumsURL = a.URL
		}
	}
	if zipURL == "" {
		return nil, fmt.Errorf("release %s has no asset %q for %s/%s", rel.TagName, zipName, u.GOOS, u.GOARCH)
	}

	fmt.Fprintf(out, "downloading %s ...\n", zipName)
	zipBytes, err := u.download(ctx, zipURL)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}

	if sumsURL != "" {
		sums, err := u.download(ctx, sumsURL)
		if err != nil {
			return nil, fmt.Errorf("download checksums: %w", err)
		}
		want, err := ParseChecksum(string(sums), zipName)
		if err != nil {
			return nil, err
		}
		got := sha256Hex(zipBytes)
		if got != want {
			return nil, fmt.Errorf("checksum mismatch for %s: got %s, want %s", zipName, got, want)
		}
		fmt.Fprintln(out, "checksum verified")
	}

	binName := BinaryName(u.GOOS)
	binBytes, err := ExtractBinary(zipBytes, binName)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(out, "installing %s ...\n", rel.TagName)
	if err := u.Apply(bytes.NewReader(binBytes)); err != nil {
		return nil, fmt.Errorf("apply update: %w", err)
	}
	return res, nil
}

func (u *Updater) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s returned %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MB cap
}

// AssetName returns the release zip name for a version and platform, matching
// release.sh's packaging convention.
func AssetName(version, goos, goarch string) string {
	return fmt.Sprintf("ninalertbot-%s-%s-%s.zip", version, goos, goarch)
}

// BinaryName returns the binary file name inside the zip for a platform.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "ninalertbot.exe"
	}
	return "ninalertbot"
}

// ParseChecksum extracts the hex digest for filename from a `shasum -a 256`
// style file ("<hex>  <filename>" per line).
func ParseChecksum(sums, filename string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// The filename may be prefixed with "./" or a path.
		name := fields[1]
		name = name[strings.LastIndexByte(name, '/')+1:]
		if name == filename {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum found for %s", filename)
}

// ExtractBinary returns the bytes of binName from a zip archive. The binary may
// live at the archive root or inside a single top-level directory.
func ExtractBinary(zipBytes []byte, binName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		base := f.Name[strings.LastIndexByte(f.Name, '/')+1:]
		if base == binName {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binName)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// isNewer reports whether latest is a strictly newer semver than current.
// A non-semver current (e.g. a dev build) is always considered older, so an
// update is offered.
func isNewer(current, latest string) bool {
	if !semver.IsValid(latest) {
		return false
	}
	if !semver.IsValid(current) {
		return true
	}
	return semver.Compare(latest, current) > 0
}
