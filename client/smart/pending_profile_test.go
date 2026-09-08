package smart

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func isolatePendingProfiles(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	if got, want := UserSettingsDir(), filepath.Join(root, "PinusSmartAWGPreview"); got != want {
		t.Fatalf("pending profile directory escaped temporary APPDATA: %q", got)
	}
}

func TestPendingProfileEncryptedRoundTripPreservesRename(t *testing.T) {
	isolatePendingProfiles(t)
	config := parseTestProfile(t)
	config.Name = "Renamed-synthetic-profile"
	const original = "Original-synthetic-profile"
	if err := SavePendingProfile(original, config); err != nil {
		t.Fatal(err)
	}
	encrypted, err := os.ReadFile(pendingProfilePath(original))
	if err != nil {
		t.Fatal(err)
	}
	if len(encrypted) == 0 || bytes.Contains(encrypted, []byte(config.Interface.PrivateKey.String())) || bytes.Contains(encrypted, []byte("[Interface]")) {
		t.Fatal("pending profile was not stored as encrypted data")
	}
	loaded, err := LoadPendingProfile(original)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Name != config.Name || loaded.ToWgQuick() != config.ToWgQuick() {
		t.Fatal("pending profile contents or renamed profile name changed after round trip")
	}
	if underNewName, err := LoadPendingProfile(config.Name); err != nil || underNewName != nil {
		t.Fatalf("draft must stay associated with the original profile: err=%v", err)
	}
}

func TestPendingProfileDeleteAndRecreateDoesNotReviveOldDraft(t *testing.T) {
	isolatePendingProfiles(t)
	const original = "Recreated-synthetic-profile"
	if config, err := LoadPendingProfile(original); err != nil || config != nil {
		t.Fatalf("missing draft should be empty: err=%v", err)
	}
	oldDraft := parseTestProfile(t)
	oldDraft.Name = "Old-pending-name"
	if err := SavePendingProfile(original, oldDraft); err != nil {
		t.Fatal(err)
	}
	otherDraft := parseTestProfile(t)
	otherDraft.Name = "Unrelated-synthetic-profile"
	if err := SavePendingProfile(otherDraft.Name, otherDraft); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := DeletePendingProfile(original); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(pendingProfilePath(original)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted draft still exists: err=%v", err)
	}
	if config, err := LoadPendingProfile(original); err != nil || config != nil {
		t.Fatalf("deleted draft must not reappear when its profile name is reused: err=%v", err)
	}
	newDraft := parseTestProfile(t)
	newDraft.Name = original
	newDraft.Peers[0].Endpoint.Host = "203.0.113.20"
	if err := SavePendingProfile(original, newDraft); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPendingProfile(original)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Name != newDraft.Name || loaded.ToWgQuick() != newDraft.ToWgQuick() {
		t.Fatal("recreated draft contains stale profile contents")
	}
	untouched, err := LoadPendingProfile(otherDraft.Name)
	if err != nil {
		t.Fatal(err)
	}
	if untouched == nil || untouched.ToWgQuick() != otherDraft.ToWgQuick() {
		t.Fatal("deleting one draft changed an unrelated draft")
	}
}

func TestPendingProfileRejectsWrongOwnerAndPreservesUnreadableDraft(t *testing.T) {
	isolatePendingProfiles(t)
	config := parseTestProfile(t)
	const original = "Synthetic-owner"
	if err := SavePendingProfile(original, config); err != nil {
		t.Fatal(err)
	}
	encrypted, err := os.ReadFile(pendingProfilePath(original))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"Different-synthetic-owner", encrypted},
		{"Corrupt-synthetic-draft", []byte("not a DPAPI profile")},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := pendingProfilePath(test.name)
			if err := os.WriteFile(path, test.data, 0600); err != nil {
				t.Fatal(err)
			}
			if loaded, err := LoadPendingProfile(test.name); err == nil || loaded != nil {
				t.Fatal("unreadable or wrong-owner draft was accepted")
			}
			preserved, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(preserved, test.data) {
				t.Fatalf("failed load changed the draft on disk: err=%v", err)
			}
		})
	}
}
