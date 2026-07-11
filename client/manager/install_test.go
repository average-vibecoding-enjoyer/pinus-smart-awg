package manager

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amnezia-vpn/amneziawg-windows-client/version"
)

func TestVerifyPinnedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.dll")
	content := []byte("known runtime dependency")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	expected := fmt.Sprintf("%X", digest[:])
	if err := verifyPinnedFile(path, expected, "runtime.dll"); err != nil {
		t.Fatalf("valid dependency rejected: %v", err)
	}

	if err := os.WriteFile(path, []byte("tampered runtime dependency"), 0600); err != nil {
		t.Fatal(err)
	}
	err := verifyPinnedFile(path, expected, "runtime.dll")
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Fatalf("tampered dependency error = %v", err)
	}
}

func TestVerifyWintunDLLRejectsMissingFile(t *testing.T) {
	err := verifyWintunDLL(filepath.Join(t.TempDir(), "wintun.dll"))
	if !os.IsNotExist(err) {
		t.Fatalf("missing dependency error = %v", err)
	}
}

func TestSecureInstallDirectoryIsVersioned(t *testing.T) {
	dataDirectory := filepath.Join(`C:\Program Files`, "Pinus Smart AWG Next", "Data")
	got := secureInstallDirectory(dataDirectory)
	want := filepath.Join(`C:\Program Files`, "Pinus Smart AWG Next", "Versions", version.Number)
	if got != want {
		t.Fatalf("secure install directory = %q, want %q", got, want)
	}
}

func TestRemoveOtherInstalledVersionsPreservesCurrent(t *testing.T) {
	versions := t.TempDir()
	current := filepath.Join(versions, "3.2.0")
	old := filepath.Join(versions, "3.1.2")
	for _, directory := range []string{current, old} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "PinusSmartAWG.exe"), []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeOtherInstalledVersionsFrom(versions, current); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatalf("current version was removed: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old version still exists: %v", err)
	}
}
