// Package update downloads and atomically replaces the installed executable.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const latestURL = "https://api.github.com/repos/hyuck0221/subscale/releases/latest"
const maxArchive = 100 << 20

type Updater struct {
	Client *http.Client
	APIURL string
	OS     string
	Arch   string
}
type Result struct {
	Version string
	Changed bool
}
type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func Default() Updater {
	return Updater{Client: &http.Client{Timeout: 2 * time.Minute}, APIURL: latestURL, OS: runtime.GOOS, Arch: runtime.GOARCH}
}
func (u Updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "subscale-updater")
	req.Header.Set("Accept", "application/octet-stream")
	if url == u.APIURL {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	res, e := u.Client.Do(req)
	if e != nil {
		return nil, e
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		if res.StatusCode == 404 {
			return nil, fmt.Errorf("no published release or download found at %s", url)
		}
		return nil, fmt.Errorf("download returned HTTP %d", res.StatusCode)
	}
	return res, nil
}
func (u Updater) Apply(ctx context.Context, executable, current string) (Result, error) {
	var result Result
	if (u.OS != "darwin" && u.OS != "linux") || (u.Arch != "amd64" && u.Arch != "arm64") {
		return result, fmt.Errorf("unsupported platform %s/%s", u.OS, u.Arch)
	}
	res, e := u.get(ctx, u.APIURL)
	if e != nil {
		return result, e
	}
	var rel release
	e = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&rel)
	res.Body.Close()
	if e != nil {
		return result, e
	}
	if rel.Tag == "" || !strings.HasPrefix(rel.Tag, "v") {
		return result, fmt.Errorf("invalid release version %q", rel.Tag)
	}
	result.Version = rel.Tag
	if current == rel.Tag {
		return result, nil
	}
	asset := "subscale_" + u.OS + "_" + u.Arch + ".tar.gz"
	var archiveURL, checksumURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case asset:
			archiveURL = a.URL
		case "checksums.txt":
			checksumURL = a.URL
		}
	}
	if archiveURL == "" || checksumURL == "" {
		return result, fmt.Errorf("release %s is missing %s or checksums.txt", rel.Tag, asset)
	}
	res, e = u.get(ctx, checksumURL)
	if e != nil {
		return result, e
	}
	expected, e := checksum(io.LimitReader(res.Body, 1<<20), asset)
	res.Body.Close()
	if e != nil {
		return result, e
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return result, e
	}
	info, e := os.Stat(executable)
	if e != nil {
		return result, e
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("executable is not a regular file")
	}
	// Both temporary files live beside the executable, so final rename is atomic.
	archive, e := os.CreateTemp(filepath.Dir(executable), ".subscale-download-*")
	if e != nil {
		return result, fmt.Errorf("cannot write install directory: %w", e)
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	res, e = u.get(ctx, archiveURL)
	if e != nil {
		return result, e
	}
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(res.Body, maxArchive+1))
	res.Body.Close()
	if e != nil {
		return result, e
	}
	if n > maxArchive {
		return result, fmt.Errorf("release archive exceeds size limit")
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return result, fmt.Errorf("release checksum verification failed; existing binary kept")
	}
	if _, e = archive.Seek(0, io.SeekStart); e != nil {
		return result, e
	}
	candidate, e := os.CreateTemp(filepath.Dir(executable), ".subscale-update-*")
	if e != nil {
		return result, e
	}
	defer os.Remove(candidate.Name())
	defer candidate.Close()
	if e = extract(archive, candidate); e != nil {
		return result, e
	}
	if e = candidate.Chmod(info.Mode().Perm() | 0700); e != nil {
		return result, e
	}
	if e = candidate.Sync(); e != nil {
		return result, e
	}
	if e = candidate.Close(); e != nil {
		return result, e
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, e := exec.CommandContext(checkCtx, candidate.Name(), "version").Output()
	if e != nil {
		return result, fmt.Errorf("downloaded executable cannot run: %w", e)
	}
	if strings.TrimSpace(string(output)) != "subscale "+rel.Tag {
		return result, fmt.Errorf("downloaded executable version does not match release %s", rel.Tag)
	}
	if e = os.Rename(candidate.Name(), executable); e != nil {
		return result, e
	}
	result.Changed = true
	return result, nil
}
func checksum(reader io.Reader, asset string) (string, error) {
	scanner := bufio.NewScanner(reader)
	found := ""
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || fields[1] != asset {
			continue
		}
		digest, e := hex.DecodeString(fields[0])
		if e != nil || len(digest) != sha256.Size || found != "" {
			return "", fmt.Errorf("invalid or duplicate release checksum")
		}
		found = strings.ToLower(fields[0])
	}
	if e := scanner.Err(); e != nil {
		return "", e
	}
	if found == "" {
		return "", fmt.Errorf("release checksum not found for %s", asset)
	}
	return found, nil
}
