package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func archive(t *testing.T, name, version string, kind byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\nprintf 'subscale " + version + "\\n'\n")
	h := &tar.Header{Name: name, Mode: 0755, Typeflag: kind, Size: int64(len(body))}
	if kind == tar.TypeSymlink {
		h.Size = 0
		h.Linkname = "/bin/sh"
	}
	if e := tw.WriteHeader(h); e != nil {
		t.Fatal(e)
	}
	if h.Size > 0 {
		if _, e := tw.Write(body); e != nil {
			t.Fatal(e)
		}
	}
	if e := tw.Close(); e != nil {
		t.Fatal(e)
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestApply(t *testing.T) {
	for _, tt := range []struct{ name, kind string }{
		{"success", "ok"}, {"corrupt checksum", "checksum"}, {"wrong binary version", "version"}, {"missing executable", "missing"}, {"symlink executable", "symlink"}, {"missing asset", "asset"}, {"no release", "404"}, {"already current", "current"}, {"broken download", "download"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			binaryVersion := "v0.2.0"
			if tt.kind == "version" {
				binaryVersion = "v0.1.0"
			}
			member := "subscale"
			if tt.kind == "missing" {
				member = "../subscale"
			}
			kind := byte(tar.TypeReg)
			if tt.kind == "symlink" {
				kind = tar.TypeSymlink
			}
			payload := archive(t, member, binaryVersion, kind)
			digest := sha256.Sum256(payload)
			sum := hex.EncodeToString(digest[:])
			if tt.kind == "checksum" {
				sum = strings.Repeat("0", 64)
			}
			asset := "subscale_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
			downloads := 0
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/latest":
					if tt.kind == "404" {
						http.NotFound(w, r)
						return
					}
					if r.Header.Get("Accept") != "application/vnd.github+json" {
						t.Error("metadata Accept header is wrong")
					}
					assets := []map[string]string{{"name": asset, "browser_download_url": server.URL + "/archive"}, {"name": "checksums.txt", "browser_download_url": server.URL + "/checksums"}}
					if tt.kind == "asset" {
						assets = assets[:1]
					}
					json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0", "assets": assets})
				case "/checksums":
					downloads++
					fmt.Fprintf(w, "%s  %s\n", sum, asset)
				case "/archive":
					downloads++
					if tt.kind == "download" {
						http.Error(w, "error", 503)
						return
					}
					w.Write(payload)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			exe := filepath.Join(dir, "subscale")
			original := []byte("existing executable")
			if e := os.WriteFile(exe, original, 0755); e != nil {
				t.Fatal(e)
			}
			// Updating a symlink must replace its target, preserving PATH and service links.
			link := filepath.Join(dir, "subscale-link")
			if e := os.Symlink(exe, link); e != nil {
				t.Fatal(e)
			}
			current := "v0.1.0"
			if tt.kind == "current" {
				current = "v0.2.0"
			}
			u := Updater{Client: server.Client(), APIURL: server.URL + "/latest", OS: runtime.GOOS, Arch: runtime.GOARCH}
			result, e := u.Apply(context.Background(), link, current)
			data, readErr := os.ReadFile(exe)
			if readErr != nil {
				t.Fatal(readErr)
			}
			switch tt.kind {
			case "ok":
				if e != nil || !result.Changed || result.Version != "v0.2.0" || bytes.Equal(data, original) {
					t.Fatalf("update = %+v, %v", result, e)
				}
			case "current":
				if e != nil || result.Changed || downloads != 0 {
					t.Fatalf("no-op = %+v, %v, downloads %d", result, e, downloads)
				}
				if !bytes.Equal(data, original) {
					t.Fatal("already-current update replaced executable")
				}
			default:
				if e == nil || result.Changed {
					t.Fatalf("expected failure, got %+v %v", result, e)
				}
				if !bytes.Equal(data, original) {
					t.Fatal("failed update changed existing executable")
				}
			}
			if info, e := os.Lstat(link); e != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("symlink replaced")
			}
			leftovers, e := filepath.Glob(filepath.Join(dir, ".subscale-*"))
			if e != nil || len(leftovers) != 0 {
				t.Fatalf("temporary files left: %v %v", leftovers, e)
			}
		})
	}
}
func TestChecksumRejectsMissingAndDuplicate(t *testing.T) {
	for _, input := range []string{"", strings.Repeat("0", 64) + "  subscale.tar.gz\n" + strings.Repeat("0", 64) + "  subscale.tar.gz\n", "not-a-digest subscale.tar.gz"} {
		if _, e := checksum(strings.NewReader(input), "subscale.tar.gz"); e == nil {
			t.Error("invalid checksum accepted")
		}
	}
}
