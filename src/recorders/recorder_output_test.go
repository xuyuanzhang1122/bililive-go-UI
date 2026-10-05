package recorders

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasRecordedOutput(t *testing.T) {
	dir := t.TempDir()
	fileName := filepath.Join(dir, "[2026-10-04 22-50-41][host][room].flv")

	if hasRecordedOutput(fileName) {
		t.Fatal("missing file must not count as recorded output")
	}
	if err := os.WriteFile(fileName, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if hasRecordedOutput(fileName) {
		t.Fatal("empty file must not count as recorded output")
	}
	if err := os.WriteFile(fileName, []byte("FLV"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasRecordedOutput(fileName) {
		t.Fatal("killed recording with data must still be post-processed")
	}

	partOnly := filepath.Join(dir, "part.flv")
	if err := os.WriteFile(filepath.Join(dir, "part_PART000.flv"), []byte("FLV"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasRecordedOutput(partOnly) {
		t.Fatal("bililive-recorder segment with data must count as recorded output")
	}
}
