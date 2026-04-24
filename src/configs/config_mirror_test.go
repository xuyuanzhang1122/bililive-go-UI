package configs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStartupConfigFileUsesPortableWhenNoRequested(t *testing.T) {
	dir := t.TempDir()
	portable := filepath.Join(dir, "portable.yml")
	t.Setenv(portableConfigEnv, portable)
	if err := os.WriteFile(portable, []byte("rpc:\n  enable: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, msg, err := ResolveStartupConfigFile("")
	if err != nil {
		t.Fatal(err)
	}
	if got != portable {
		t.Fatalf("ResolveStartupConfigFile() = %q, want %q", got, portable)
	}
	if !strings.Contains(msg, "系统默认配置镜像") {
		t.Fatalf("message = %q, want mention portable mirror", msg)
	}
}

func TestResolveStartupConfigFileRestoresMissingRequested(t *testing.T) {
	dir := t.TempDir()
	portable := filepath.Join(dir, "portable.yml")
	requested := filepath.Join(dir, "missing", "config.yml")
	t.Setenv(portableConfigEnv, portable)
	want := []byte("interval: 30\n")
	if err := os.WriteFile(portable, want, 0644); err != nil {
		t.Fatal(err)
	}

	got, _, err := ResolveStartupConfigFile(requested)
	if err != nil {
		t.Fatal(err)
	}
	if got != requested {
		t.Fatalf("ResolveStartupConfigFile() = %q, want %q", got, requested)
	}
	data, err := os.ReadFile(requested)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Fatalf("restored config = %q, want %q", string(data), string(want))
	}
}

func TestMirrorConfigToPortable(t *testing.T) {
	dir := t.TempDir()
	portable := filepath.Join(dir, "portable.yml")
	source := filepath.Join(dir, "source.yml")
	t.Setenv(portableConfigEnv, portable)
	data := []byte("live_rooms: []\n")

	if err := MirrorConfigToPortable(source, data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(portable)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("portable config = %q, want %q", string(got), string(data))
	}
}
