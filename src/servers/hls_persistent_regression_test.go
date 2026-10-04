package servers

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentHLSSourceRenameKeepsIdentityAndRejectsReplacement(t *testing.T) {
	_, cat, e, m, _ := persistentHLSFixture(t)
	renamed := filepath.Join(filepath.Dir(m.SourcePath), "renamed.flv")
	require.NoError(t, os.Rename(m.SourcePath, renamed))
	require.NoError(t, os.WriteFile(m.SourcePath, []byte("new unrelated recording"), 0600))
	require.NoError(t, cat.Reconcile(context.Background()))
	asset, err := cat.FindHLS(context.Background(), e.RecordingID, e.SourceVersion+"-hls-source-v1")
	require.NoError(t, err)
	_, err = verifyHLSManifest(httptest.NewRequest("GET", "/", nil), cat, asset)
	require.NoError(t, err, "同录播及源版本的 HLS 应跟随目录改名")
	require.NoError(t, os.Remove(renamed))
	require.NoError(t, os.WriteFile(renamed, []byte("another recording"), 0600))
	_, err = verifyHLSManifest(httptest.NewRequest("GET", "/", nil), cat, asset)
	require.Error(t, err, "目录未重新核验时也不能按同名猜来源")
}
