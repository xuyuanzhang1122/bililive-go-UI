package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSunVersionOrdering(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		want        int
	}{
		{"sun", "v2.1.2-rc.3", 1}, {"v2.1.1", "sun", -1},
		{"sun", "3.0.0", 0}, {"sun", "sun", 0},
		{"sun", "v3.0.1", -1}, {"v3.0.0-rc.1", "sun", -1},
	} {
		t.Run(tc.left+"_"+tc.right, func(t *testing.T) {
			got, err := CompareVersions(tc.left, tc.right)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSunUpdateSelection(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		wantUpdate      bool
	}{
		{"v2.1.2-rc.3", "sun", true}, {"sun", "sun", false},
		{"sun", "v2.1.1", false}, {"sun", "v3.0.1", true},
		{"v3.0.1", "sun", false},
	} {
		t.Run(tc.current+"_"+tc.latest, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode([]githubRelease{{
					TagName: tc.latest, Body: "sun 正式大版本", Assets: []githubAsset{{
						Name:               "bililive-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz",
						BrowserDownloadURL: "https://example.com/" + tc.latest + "/binary.tar.gz",
					}},
				}})
			}))
			defer server.Close()
			checker := NewChecker(tc.current)
			checker.SetMirrorURL("")
			checker.SetReleaseURL(server.URL)
			info, err := checker.CheckForUpdate(false)
			require.NoError(t, err)
			if !tc.wantUpdate {
				require.Nil(t, info)
				return
			}
			require.NotNil(t, info)
			require.Equal(t, tc.latest, info.TagName)
			require.Equal(t, "https://example.com/"+tc.latest+"/binary.tar.gz", info.DownloadURLs[0])
			require.False(t, info.Prerelease)
		})
	}
}
