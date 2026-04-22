package tools

import (
	"encoding/json"
	"testing"
)

func TestAddDownloadFallbacksExpandsGitHubURLs(t *testing.T) {
	input := []byte(`{
		"demo": {
			"v1": {
				"downloadUrl": "https://github.com/example/project/releases/download/v1/tool.zip",
				"pathToEntry": "tool"
			}
		}
	}`)

	output, err := addDownloadFallbacks(input)
	if err != nil {
		t.Fatalf("addDownloadFallbacks() error = %v", err)
	}

	var parsed map[string]map[string]struct {
		DownloadURL []string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	got := parsed["demo"]["v1"].DownloadURL
	want := []string{
		"https://github.com/example/project/releases/download/v1/tool.zip",
		"https://gh-proxy.com/https://github.com/example/project/releases/download/v1/tool.zip",
		"https://github.moeyy.xyz/https://github.com/example/project/releases/download/v1/tool.zip",
	}
	assertStringSlice(t, got, want)
}

func TestAddDownloadFallbacksExpandsNestedOSArchURLs(t *testing.T) {
	input := []byte(`{
		"demo": {
			"v1": {
				"downloadUrl": {
					"linux": {
						"amd64": [
							"https://github.com/example/project/releases/download/v1/linux.zip"
						]
					}
				},
				"pathToEntry": "tool"
			}
		}
	}`)

	output, err := addDownloadFallbacks(input)
	if err != nil {
		t.Fatalf("addDownloadFallbacks() error = %v", err)
	}

	var parsed map[string]map[string]struct {
		DownloadURL map[string]map[string][]string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	got := parsed["demo"]["v1"].DownloadURL["linux"]["amd64"]
	want := []string{
		"https://github.com/example/project/releases/download/v1/linux.zip",
		"https://gh-proxy.com/https://github.com/example/project/releases/download/v1/linux.zip",
		"https://github.moeyy.xyz/https://github.com/example/project/releases/download/v1/linux.zip",
	}
	assertStringSlice(t, got, want)
}

func TestAddDownloadFallbacksExpandsRemoteProxyUpstream(t *testing.T) {
	input := []byte(`{
		"demo": {
			"v1": {
				"downloadUrl": [
					"https://bililive-go.com/remotetools/download?downloadurl=https://github.com/example/project/releases/download/v1/tool.zip"
				],
				"pathToEntry": "tool"
			}
		}
	}`)

	output, err := addDownloadFallbacks(input)
	if err != nil {
		t.Fatalf("addDownloadFallbacks() error = %v", err)
	}

	var parsed map[string]map[string]struct {
		DownloadURL []string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	got := parsed["demo"]["v1"].DownloadURL
	want := []string{
		"https://bililive-go.com/remotetools/download?downloadurl=https://github.com/example/project/releases/download/v1/tool.zip",
		"https://github.com/example/project/releases/download/v1/tool.zip",
		"https://gh-proxy.com/https://github.com/example/project/releases/download/v1/tool.zip",
		"https://github.moeyy.xyz/https://github.com/example/project/releases/download/v1/tool.zip",
	}
	assertStringSlice(t, got, want)
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}
