package servers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalUIVersionReadsBuiltMetadata(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "version.json"), []byte(`{"uiVersion":"sun"}`), 0600))
	t.Setenv("BILILIVE_WEBAPP_PATH", dir)
	require.Equal(t, "sun", getLocalUIVersion())
}
