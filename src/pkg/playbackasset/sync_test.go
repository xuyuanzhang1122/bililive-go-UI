package playbackasset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicationParentPolicyPreservesIOErrors(t *testing.T) {
	for _, flush := range []bool{true, false} {
		dir := t.TempDir()
		require.NoError(t, syncPublicationParent(filepath.Join(dir, "asset.mp4"), flush))
		require.Error(t, syncPublicationParent(filepath.Join(dir, "missing", "asset.mp4"), flush))
		parentFile := filepath.Join(dir, "file")
		require.NoError(t, os.WriteFile(parentFile, []byte("file"), 0600))
		require.Error(t, syncPublicationParent(filepath.Join(parentFile, "asset.mp4"), flush))
	}
}
