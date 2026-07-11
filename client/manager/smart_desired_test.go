/* SPDX-License-Identifier: MIT */

package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
)

func selectedDesiredSettings() smart.RoutingSettings {
	settings := smart.DefaultSettings()
	settings.Mode = smart.ModeSelected
	return settings
}

func TestSmartDesiredRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desired.json")
	want := smartDesiredState{TunnelName: "Phone", Settings: selectedDesiredSettings()}
	if err := writeSmartDesiredFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, exists, err := readSmartDesiredFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || got.Version != smartDesiredVersion || got.TunnelName != want.TunnelName {
		t.Fatalf("desired state = %#v, exists=%v", got, exists)
	}
	if got.Settings.Mode != smart.ModeSelected || got.UpdatedAt.IsZero() {
		t.Fatalf("normalized desired state = %#v", got)
	}
}

func TestSmartDesiredRejectsNativeMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desired.json")
	err := writeSmartDesiredFile(path, smartDesiredState{
		TunnelName: "Phone",
		Settings:   smart.RoutingSettings{Mode: smart.ModeAll},
	})
	if err == nil {
		t.Fatal("native all-traffic state was persisted for smart recovery")
	}
}

func TestSmartDesiredRejectsOversizedAndCorruptFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desired.json")
	if err := os.WriteFile(path, make([]byte, smartDesiredMaxSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readSmartDesiredFile(path); err == nil {
		t.Fatal("oversized desired state was accepted")
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"tunnel_name":"Phone"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readSmartDesiredFile(path); err == nil {
		t.Fatal("invalid native desired state was accepted")
	}
}

func TestSmartDesiredMissingFile(t *testing.T) {
	_, exists, err := readSmartDesiredFile(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || exists {
		t.Fatalf("missing desired state: exists=%v err=%v", exists, err)
	}
}
