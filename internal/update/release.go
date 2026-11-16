package update

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var releaseTag = regexp.MustCompile(`^v[0-9][0-9A-Za-z._+-]*$`)

// GitHub's public /releases/latest redirects to /releases/tag/VERSION.
// Resolve it once, then pin both downloads to that tag to avoid mixing releases
// if a new version is published while an update is in progress.
func (u Updater) releaseURLs(ctx context.Context, asset string) (version, archiveURL, checksumURL string, err error) {
	latest, err := url.Parse(u.LatestURL)
	if err != nil || latest == nil || !strings.HasSuffix(latest.Path, "/latest") || latest.Host == "" || (latest.Scheme != "https" && latest.Scheme != "http") || latest.RawQuery != "" || latest.Fragment != "" || latest.User != nil {
		return "", "", "", fmt.Errorf("invalid latest release URL %q", u.LatestURL)
	}
	res, err := u.get(ctx, u.LatestURL)
	if err != nil {
		return "", "", "", err
	}
	res.Body.Close()
	final := res.Request.URL
	basePath := strings.TrimSuffix(latest.Path, "/latest")
	prefix := basePath + "/tag/"
	if final.Scheme != latest.Scheme || final.Host != latest.Host || !strings.HasPrefix(final.Path, prefix) || final.RawQuery != "" || final.Fragment != "" || final.User != nil {
		return "", "", "", fmt.Errorf("unexpected latest release destination %s", final.Redacted())
	}
	version = strings.TrimPrefix(final.Path, prefix)
	if !releaseTag.MatchString(version) {
		return "", "", "", fmt.Errorf("invalid release version %q", version)
	}
	base := *latest
	base.Path = basePath + "/download/" + version + "/"
	base.RawPath = ""
	return version, base.String() + asset, base.String() + "checksums.txt", nil
}
