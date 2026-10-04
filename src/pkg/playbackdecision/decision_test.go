package playbackdecision

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func fixtureCaps() Capabilities {
	return Capabilities{Containers: []string{"mp4", "hls"}, Protocols: []string{"file", "hls"}, Video: []VideoCapability{{Codec: "h264", Profiles: []string{"baseline", "main", "high"}, MaxLevel: 42, MaxWidth: 1920, MaxHeight: 1080, MaxFPS: 60, MaxBitDepth: 8}}, Audio: []AudioCapability{{Codec: "aac", Profiles: []string{"lc"}, MaxChannels: 2, MaxSampleRate: 48000}}}
}
func fixtureMedia() Media {
	return Media{Container: "mp4", Protocol: "file", Video: Video{Codec: "h264", Profile: "main", Level: 41, Width: 1280, Height: 720, FPS: 30, BitDepth: 8}, Audio: &Audio{Codec: "aac", Profile: "lc", Channels: 2, SampleRate: 48000}}
}
func TestDecisionRejectsUnsupportedMediaProperties(t *testing.T) {
	cases := []struct {
		name, want string
		change     func(*Media)
	}{
		{"supported", "direct", func(*Media) {}},
		{"container", "remux", func(m *Media) { m.Container = "flv" }},
		{"protocol", "remux", func(m *Media) { m.Protocol = "rtmp" }},
		{"audio codec", "audio-transcode", func(m *Media) { m.Audio.Codec = "opus" }},
		{"audio profile", "audio-transcode", func(m *Media) { m.Audio.Profile = "he-aac" }},
		{"channels", "audio-transcode", func(m *Media) { m.Audio.Channels = 6 }},
		{"sample rate", "audio-transcode", func(m *Media) { m.Audio.SampleRate = 96000 }},
		{"codec", "video-transcode", func(m *Media) { m.Video.Codec = "hevc" }},
		{"profile", "video-transcode", func(m *Media) { m.Video.Profile = "high 10" }},
		{"level", "video-transcode", func(m *Media) { m.Video.Level = 51 }},
		{"width", "video-transcode", func(m *Media) { m.Video.Width = 3840 }},
		{"height", "video-transcode", func(m *Media) { m.Video.Height = 2160 }},
		{"fps", "video-transcode", func(m *Media) { m.Video.FPS = 120 }},
		{"depth", "video-transcode", func(m *Media) { m.Video.BitDepth = 10 }},
		{"hdr", "video-transcode", func(m *Media) { m.Video.HDR = true }},
		{"unknown level", "video-transcode", func(m *Media) { m.Video.Level = 0 }},
		{"unknown depth", "video-transcode", func(m *Media) { m.Video.BitDepth = 0 }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			m := fixtureMedia()
			tt.change(&m)
			d := Decide(m, fixtureCaps())
			require.Equal(t, tt.want, d.Mode)
			require.Equal(t, tt.want == "direct", d.Compatible)
		})
	}
	m := fixtureMedia()
	m.Audio = nil
	require.True(t, Decide(m, fixtureCaps()).Compatible)
	m.Video.Profile = "Constrained Baseline"
	require.True(t, Decide(m, fixtureCaps()).Compatible)
}
func TestCapabilitiesMustStateFiniteSupportedLimits(t *testing.T) {
	c := fixtureCaps()
	require.NoError(t, Validate(c))
	c.Video[0].MaxFPS = 0
	require.Error(t, Validate(c))
	c = fixtureCaps()
	c.Video[0].MaxBitDepth = 0
	require.Error(t, Validate(c))
	c = fixtureCaps()
	c.Protocols = []string{"unknown"}
	require.Error(t, Validate(c))
	c = fixtureCaps()
	c.Video[0].Profiles = nil
	require.Error(t, Validate(c))
	c = fixtureCaps()
	c.Containers = nil
	require.Error(t, Validate(c))
	c = fixtureCaps()
	c.Audio[0].MaxChannels = -1
	require.Error(t, Validate(c))
}
