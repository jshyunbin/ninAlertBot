package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.2.0", "v0.1.0", false},
		{"v0.2.0", "v0.10.0", true},
		{"dev", "v0.1.0", true},      // non-semver current -> update offered
		{"v0.1.0", "garbage", false}, // invalid latest -> no update
	}
	for _, c := range cases {
		if got := isNewer(c.current, c.latest); got != c.want {
			t.Errorf("isNewer(%q,%q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestAssetAndBinaryName(t *testing.T) {
	if got, want := AssetName("v0.3.0", "windows", "amd64"), "ninalertbot-v0.3.0-windows-amd64.zip"; got != want {
		t.Errorf("AssetName = %q, want %q", got, want)
	}
	if got := BinaryName("windows"); got != "ninalertbot.exe" {
		t.Errorf("BinaryName(windows) = %q", got)
	}
	if got := BinaryName("linux"); got != "ninalertbot" {
		t.Errorf("BinaryName(linux) = %q", got)
	}
}

func TestParseChecksum(t *testing.T) {
	sums := "aaaa  ./ninalertbot-v0.3.0-linux-amd64.zip\nbbbb  ninalertbot-v0.3.0-windows-amd64.zip\n"
	got, err := ParseChecksum(sums, "ninalertbot-v0.3.0-windows-amd64.zip")
	if err != nil || got != "bbbb" {
		t.Fatalf("ParseChecksum = %q, %v; want bbbb", got, err)
	}
	if _, err := ParseChecksum(sums, "missing.zip"); err == nil {
		t.Error("expected error for missing file")
	}
}

func makeZip(t *testing.T, binName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Put the binary inside a top-level directory, like release.sh does.
	w, err := zw.Create("ninalertbot-v0.3.0-linux-amd64/" + binName)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(content)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	z := makeZip(t, "ninalertbot", []byte("BINARY"))
	got, err := ExtractBinary(z, "ninalertbot")
	if err != nil {
		t.Fatalf("ExtractBinary error = %v", err)
	}
	if string(got) != "BINARY" {
		t.Errorf("got %q, want BINARY", got)
	}
	if _, err := ExtractBinary(z, "nope.exe"); err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestUpdateEndToEnd(t *testing.T) {
	binContent := []byte("NEW-BINARY-v0.3.0")
	zipBytes := makeZip(t, "ninalertbot", binContent)
	zipName := "ninalertbot-v0.3.0-linux-amd64.zip"
	sum := sha256.Sum256(zipBytes)
	sumsFile := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), zipName)

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/jshyunbin/ninAlertBot/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
			"tag_name": "v0.3.0",
			"html_url": "https://example/releases/v0.3.0",
			"assets": [
				{"name": %q, "browser_download_url": "%s/dl/zip"},
				{"name": "SHA256SUMS.txt", "browser_download_url": "%s/dl/sums"}
			]
		}`, zipName, srv.URL, srv.URL)
	})
	mux.HandleFunc("/dl/zip", func(w http.ResponseWriter, r *http.Request) { w.Write(zipBytes) })
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sumsFile)) })
	srv = httptest.NewServer(mux)
	defer srv.Close()

	var applied []byte
	u := &Updater{
		Owner: "jshyunbin", Repo: "ninAlertBot",
		Current: "v0.2.0", APIBase: srv.URL,
		HTTP:   srv.Client(),
		GOOS:   "linux",
		GOARCH: "amd64",
		Apply: func(r io.Reader) error {
			var err error
			applied, err = io.ReadAll(r)
			return err
		},
	}

	res, err := u.Update(context.Background(), io.Discard)
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}
	if !res.HasUpdate || res.Latest != "v0.3.0" {
		t.Errorf("unexpected result %+v", res)
	}
	if !bytes.Equal(applied, binContent) {
		t.Errorf("applied = %q, want %q", applied, binContent)
	}
}

func TestUpdateNoNewerVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/jshyunbin/ninAlertBot/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v0.2.0", "assets": []}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	applyCalled := false
	u := &Updater{
		Owner: "jshyunbin", Repo: "ninAlertBot",
		Current: "v0.2.0", APIBase: srv.URL, HTTP: srv.Client(),
		GOOS: "linux", GOARCH: "amd64",
		Apply: func(r io.Reader) error { applyCalled = true; return nil },
	}
	res, err := u.Update(context.Background(), io.Discard)
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}
	if res.HasUpdate {
		t.Error("HasUpdate should be false")
	}
	if applyCalled {
		t.Error("Apply must not be called when already up to date")
	}
}

func TestUpdateChecksumMismatch(t *testing.T) {
	zipBytes := makeZip(t, "ninalertbot", []byte("payload"))
	zipName := "ninalertbot-v0.3.0-linux-amd64.zip"
	badSums := "deadbeef  " + zipName + "\n"

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/jshyunbin/ninAlertBot/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v0.3.0","assets":[
			{"name":%q,"browser_download_url":"%s/dl/zip"},
			{"name":"SHA256SUMS.txt","browser_download_url":"%s/dl/sums"}]}`, zipName, srv.URL, srv.URL)
	})
	mux.HandleFunc("/dl/zip", func(w http.ResponseWriter, r *http.Request) { w.Write(zipBytes) })
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(badSums)) })
	srv = httptest.NewServer(mux)
	defer srv.Close()

	applyCalled := false
	u := &Updater{
		Owner: "jshyunbin", Repo: "ninAlertBot", Current: "v0.2.0",
		APIBase: srv.URL, HTTP: srv.Client(), GOOS: "linux", GOARCH: "amd64",
		Apply: func(r io.Reader) error { applyCalled = true; return nil },
	}
	if _, err := u.Update(context.Background(), io.Discard); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if applyCalled {
		t.Error("Apply must not run on checksum mismatch")
	}
}
