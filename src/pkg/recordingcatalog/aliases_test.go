package recordingcatalog

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogAliasRejectsSameSizeAndTimeReplacement(t *testing.T) {
	s, _, source := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	renamed := filepath.Join(filepath.Dir(source), "renamed.flv")
	require.NoError(t, os.Rename(source, renamed))
	require.NoError(t, s.Reconcile(ctx))
	before, err := os.Stat(renamed)
	require.NoError(t, err)
	require.NoError(t, os.Rename(renamed, renamed+".saved"))
	require.NoError(t, os.WriteFile(renamed, []byte("other recording!"), 0600))
	require.NoError(t, os.Chtimes(renamed, before.ModTime(), before.ModTime()))
	after, err := os.Stat(renamed)
	require.NoError(t, err)
	require.Equal(t, before.Size(), after.Size())
	_, err = s.ResolveAlias(ctx, "抖音/主播/clip.flv")
	require.Error(t, err, "尺寸和时间一样也不能读取替换后的其他文件")
}

func TestCatalogAliasRejectsParentSymlinkOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 符号链接需要额外权限")
	}
	s, _, source := catalogFixture(t)
	ctx := context.Background()
	require.NoError(t, s.Reconcile(ctx))
	require.NoError(t, os.Rename(source, filepath.Join(filepath.Dir(source), "renamed.flv")))
	require.NoError(t, s.Reconcile(ctx))
	host := filepath.Dir(source)
	outside := filepath.Join(t.TempDir(), "host")
	require.NoError(t, os.Rename(host, outside))
	require.NoError(t, os.Symlink(outside, host))
	_, err := s.ResolveAlias(ctx, "抖音/主播/clip.flv")
	require.Error(t, err, "身份相同的文件移出根目录后也不能经链接越权")
}
