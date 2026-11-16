package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReleaseRedirectValidation(t *testing.T) {
	for _, tc := range []struct {
		name, destination string
		valid             bool
	}{
		{"version", "/releases/tag/v1.2.3", true},
		{"prerelease", "/releases/tag/v1.2.3-rc.1", true},
		{"no version prefix", "/releases/tag/latest", false},
		{"unexpected page", "/login", false},
		{"different repository", "/other/releases/tag/v1.2.3", false},
		{"nested tag", "/releases/tag/v1.2/extra", false},
		{"encoded slash", "/releases/tag/v1.2%2Fextra", false},
		{"query", "/releases/tag/v1.2.3?unexpected=1", false},
		{"no redirect", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/releases/latest" && tc.destination != "" {
					http.Redirect(w, r, tc.destination, http.StatusFound)
					return
				}
				w.Write([]byte("page"))
			}))
			defer server.Close()
			u := Updater{Client: server.Client(), LatestURL: server.URL + "/releases/latest"}
			version, archive, sum, err := u.releaseURLs(context.Background(), "test.tar.gz")
			if (err == nil) != tc.valid {
				t.Fatalf("%q: %v", tc.destination, err)
			}
			if tc.valid {
				base := server.URL + "/releases/download/" + version + "/"
				if archive != base+"test.tar.gz" || sum != base+"checksums.txt" {
					t.Fatalf("unversioned download URLs: %s %s", archive, sum)
				}
			}
		})
	}
}

func TestReleaseRejectsForeignOrigin(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("page")) }))
	defer foreign.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/releases/tag/v1.2.3", http.StatusFound)
	}))
	defer server.Close()
	u := Updater{Client: server.Client(), LatestURL: server.URL + "/releases/latest"}
	if _, _, _, err := u.releaseURLs(context.Background(), "test.tar.gz"); err == nil {
		t.Fatal("accepted different origin")
	}
}
