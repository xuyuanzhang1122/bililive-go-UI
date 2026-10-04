package servers

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestHLSPreparedOriginalURL(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "active"}[active], func(t *testing.T) {
			cfg := lifecycleConfig(t)
			oldCfg, oldManager := configs.GetCurrentConfig(), globalHLSJobManager
			m := &hlsJobManager{maxConcurrent: 1}
			configs.SetCurrentConfig(cfg)
			globalHLSJobManager = m
			defer func() { m.Close(); configs.SetCurrentConfig(oldCfg); globalHLSJobManager = oldManager }()
			source := filepath.Join(cfg.OutPutPath, "clip.flv")
			var original *hlsJob
			if active {
				original = lifecycleJob(t, m, cfg, "clip", "hold")
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_, err := m.WaitPlayable(ctx, original)
				require.NoError(t, err)
			} else {
				require.NoError(t, os.WriteFile(source, []byte("short"), 0644))
			}
			info, err := os.Stat(source)
			require.NoError(t, err)
			target := filepath.Join(cfg.OutPutPath, "clip.mp4")
			require.NoError(t, os.WriteFile(target, []byte("short"), 0644))
			require.NoError(t, playbackasset.Save(source, target, info))
			require.NoError(t, os.Remove(source))
			res := httptest.NewRecorder()
			getHLSPlaylist(res, mux.SetURLVars(httptest.NewRequest("GET", "/hls/clip.flv/playlist.m3u8", nil), map[string]string{"path": "clip.flv/playlist.m3u8"}))
			require.Equal(t, 200, res.Code, res.Body.String())
			if active {
				require.Contains(t, res.Body.String(), original.key)
				_, err = os.Stat(target + ".starts")
				require.True(t, os.IsNotExist(err))
				require.NoError(t, os.WriteFile(source+".release", nil, 0644))
			}
			var key string
			m.mutex.RLock()
			for k := range m.jobs {
				key = k
			}
			m.mutex.RUnlock()
			segment := httptest.NewRecorder()
			getHLSSegment(segment, mux.SetURLVars(httptest.NewRequest("GET", "/segment", nil), map[string]string{"cache_key": key, "segment": "seg_00000.ts"}))
			require.Equal(t, 200, segment.Code)
			require.Equal(t, "segment payload", segment.Body.String())
		})
	}
}
