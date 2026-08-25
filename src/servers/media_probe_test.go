package servers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHLSVideoProbeKeepsCompatibleH264AACInCopyMode(t *testing.T) {
	mode := hlsEncodeModeForProbe(mediaProbe{
		Streams: []mediaProbeStream{
			{CodecType: "video", CodecName: "h264"},
			{CodecType: "audio", CodecName: "aac"},
		},
	})
	assert.Equal(t, hlsEncodeCopy, mode)
}

func TestHLSVideoProbeTranscodesUnsupportedVideoOrAudio(t *testing.T) {
	cases := []struct {
		name    string
		streams []mediaProbeStream
	}{
		{name: "hevc", streams: []mediaProbeStream{{CodecType: "video", CodecName: "hevc"}}},
		{name: "vp9", streams: []mediaProbeStream{{CodecType: "video", CodecName: "vp9"}}},
		{name: "ac3", streams: []mediaProbeStream{{CodecType: "video", CodecName: "h264"}, {CodecType: "audio", CodecName: "ac3"}}},
		{name: "no-video", streams: []mediaProbeStream{{CodecType: "audio", CodecName: "aac"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, hlsEncodeTranscode, hlsEncodeModeForProbe(mediaProbe{Streams: testCase.streams}))
		})
	}
}

func TestHLSTranscodeArgsSelectsCrossPlatformH264AACProfile(t *testing.T) {
	args := hlsTranscodeArgs("source.mkv", "seg_%05d.ts", "index.m3u8")
	assert.Contains(t, args, "libx264")
	assert.Contains(t, args, "yuv420p")
	assert.Contains(t, args, "aac")
	assert.Contains(t, args, "-map")
	assert.Contains(t, args, "0:a:0?")
}

func TestProbeMediaParsesFFprobeJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses a POSIX helper")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "ffprobe")
	require.NoError(t, os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s' '{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\"},{\"codec_type\":\"audio\",\"codec_name\":\"aac\"}],\"format\":{\"format_name\":\"matroska\"}}'\n"), 0o755))

	probe, err := probeMedia(context.Background(), helper, "source.mkv")
	require.NoError(t, err)
	assert.Equal(t, "matroska", probe.Format.FormatName)
	assert.Equal(t, hlsEncodeCopy, hlsEncodeModeForProbe(probe))
}
