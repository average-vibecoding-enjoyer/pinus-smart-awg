package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// Audit regression probe: calls the actual file reader, using only synthetic
// files under t.TempDir. Does not initialize UI, IPC, VPN or user settings.
func TestAuditReadProfileFilesReportsPartialReadFailure(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.conf")
	if err := os.WriteFile(validPath, []byte("synthetic config; parser is not invoked"), 0600); err != nil {
		t.Fatal(err)
	}
	missingPath := filepath.Join(dir, "missing.conf")
	profiles, err := readProfileFiles([]string{validPath, missingPath})
	if len(profiles) != 1 {
		t.Fatalf("expected readable profile to survive partial failure, got %d", len(profiles))
	}
	if err == nil {
		t.Fatal("partial import silently lost missing.conf: readable result must also carry an error")
	}
}

func TestAuditReadProfileFilesReportsPartialOversizedInput(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.conf")
	oversizedPath := filepath.Join(dir, "oversized.conf")
	if err := os.WriteFile(validPath, []byte("synthetic config; parser is not invoked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oversizedPath, make([]byte, maxImportedProfileSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	profiles, err := readProfileFiles([]string{validPath, oversizedPath})
	if len(profiles) != 1 {
		t.Fatalf("expected only readable, bounded profile, got %d", len(profiles))
	}
	if err == nil {
		t.Fatal("partial import silently lost oversized.conf: rejection must be reported")
	}
}
