package stages

import (
	"encoding/json"
	"fmt"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistentHLSReuseAfterSourceRename(t *testing.T) {
	ctx, input, _, _ := preparationFixture(t, "libx264", "aac")
	_, err := (&PreparePlaybackStage{}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	_, err = (&PrepareHLSStage{profiles: []string{"source"}}).Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	renamed := filepath.Join(filepath.Dir(input.Path), "renamed.mkv")
	require.NoError(t, os.Rename(input.Path, renamed))
	input.Path = renamed
	require.NoError(t, ctx.Catalog.Reconcile(ctx.Ctx))
	stage := &PrepareHLSStage{profiles: []string{"source"}}
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.NoError(t, err)
	require.Empty(t, stage.GetCommands())
}

func TestHLSRestartCleansOnlyOwnedUncommittedBuildsBeforeAdmission(t *testing.T) {
	ctx, input, store, _ := preparationFixture(t, "libx264", "aac")
	e, err := ctx.Catalog.PreparationEntry(ctx.Ctx, input.Path, input.PreparationSourceVersion)
	require.NoError(t, err)
	dir := filepath.Join(ctx.Catalog.AssetRoot(), e.RecordingID, e.SourceVersion+"-hls-source-v1")
	require.NoError(t, os.MkdirAll(dir, 0755))
	owner := hlsasset.Manifest{Version: 1, RecordingID: e.RecordingID, SourceVersion: e.SourceVersion, SourcePath: input.Path, Profile: "source"}
	create := func(name string, m *hlsasset.Manifest) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.Mkdir(path, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(path, "partial.ts"), []byte("orphan media"), 0600))
		if m != nil {
			previous := *m
			previous.DataDir = name
			data, err := json.Marshal(previous)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(path, ".build-owner.json"), data, 0600))
		}
		return path
	}
	ownerJSON, err := json.Marshal(owner)
	require.NoError(t, err)
	child := exec.Command(os.Args[0], "-test.run=^TestHLSBuildExitFixture$")
	child.Env = append(os.Environ(), "BILIVE_T4_HLS_CRASH_ROOT="+dir, "BILIVE_T4_HLS_CRASH_OWNER="+string(ownerJSON))
	output, err := child.CombinedOutput()
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 72, exit.ExitCode(), string(output))
	abandoned := strings.TrimSpace(string(output))
	require.DirExists(t, abandoned)
	unknown := create(".hls-build-unknown", nil)
	other := owner
	other.Profile = "720p"
	otherDir := create(".hls-build-other", &other)
	stage := &PrepareHLSStage{profiles: []string{"source"}, runner: &PreparePlaybackStage{freeBytes: func(string) (uint64, error) {
		require.NoDirExists(t, abandoned, "重启必须先释放本任务的未发布媒体")
		require.DirExists(t, unknown)
		require.DirExists(t, otherDir)
		return 0, nil
	}}}
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	var paused *pipeline.PreparationPausedError
	require.ErrorAs(t, err, &paused)
	active := create(".hls-build-active", &owner)
	key := pipeline.PreparationKey{RecordingID: e.RecordingID, SourceVersion: e.SourceVersion, ProfileVersion: hlsasset.ProfileVersion("source")}
	require.NoError(t, store.ClaimPreparation(ctx.Ctx, key))
	_, err = stage.Execute(ctx, []pipeline.FileInfo{input})
	require.ErrorAs(t, err, &paused)
	require.DirExists(t, active, "未认领执行权不能清理活跃构建")
}

func TestHLSBuildExitFixture(t *testing.T) {
	root := os.Getenv("BILIVE_T4_HLS_CRASH_ROOT")
	if root == "" {
		t.Skip("仅用于故障注入子进程")
	}
	var owner hlsasset.Manifest
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("BILIVE_T4_HLS_CRASH_OWNER")), &owner))
	data, err := hlsasset.NewBuild(root, owner)
	require.NoError(t, err)
	defer os.RemoveAll(data)
	require.NoError(t, os.WriteFile(filepath.Join(data, "partial.ts"), make([]byte, 1<<20), 0600))
	fmt.Println(data)
	os.Exit(72)
}
