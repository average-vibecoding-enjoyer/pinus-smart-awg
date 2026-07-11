/* SPDX-License-Identifier: MIT */

package ui

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxn/walk"

	"github.com/amnezia-vpn/amneziawg-windows-client/manager"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
)

func TestHasSelectedVPNRoute(t *testing.T) {
	settings := smart.DefaultSettings()
	settings.SelectedApps = nil
	if hasSelectedVPNRoute(settings) {
		t.Fatal("empty selected mode unexpectedly has a VPN route")
	}
	settings.CustomRules = []smart.CustomRule{{Enabled: true, Target: smart.TargetDirect}}
	if hasSelectedVPNRoute(settings) {
		t.Fatal("direct-only rule unexpectedly counts as a VPN route")
	}
	settings.CustomRules = append(settings.CustomRules, smart.CustomRule{Enabled: true, Target: smart.TargetVPN})
	if !hasSelectedVPNRoute(settings) {
		t.Fatal("enabled VPN rule was not detected")
	}
}

func TestChooseActiveProfilePrefersSavedSelection(t *testing.T) {
	profiles := []profileInfo{
		{Tunnel: manager.Tunnel{Name: "Alpha"}, State: manager.TunnelStarted},
		{Tunnel: manager.Tunnel{Name: "Beta"}, State: manager.TunnelStarting},
		{Tunnel: manager.Tunnel{Name: "Stopped"}, State: manager.TunnelStopped},
	}
	name, count := chooseActiveProfile(profiles, "Beta")
	if name != "Beta" || count != 2 {
		t.Fatalf("active selection = %q/%d, want Beta/2", name, count)
	}
}

func TestShouldDisconnectForEmptySelectedMode(t *testing.T) {
	tests := []struct {
		name       string
		settings   smart.RoutingSettings
		disconnect bool
	}{
		{
			name: "selected without routes",
			settings: smart.RoutingSettings{
				Mode: smart.ModeSelected,
			},
			disconnect: true,
		},
		{
			name: "selected with direct-only rule",
			settings: smart.RoutingSettings{
				Mode: smart.ModeSelected,
				CustomRules: []smart.CustomRule{{
					Enabled: true,
					Target:  smart.TargetDirect,
				}},
			},
			disconnect: true,
		},
		{
			name: "selected with service",
			settings: smart.RoutingSettings{
				Mode:         smart.ModeSelected,
				SelectedApps: []string{"discord"},
			},
		},
		{
			name: "selected with VPN rule",
			settings: smart.RoutingSettings{
				Mode: smart.ModeSelected,
				CustomRules: []smart.CustomRule{{
					Enabled: true,
					Target:  smart.TargetVPN,
				}},
			},
		},
		{
			name: "all mode without selections",
			settings: smart.RoutingSettings{
				Mode: smart.ModeAll,
			},
		},
		{
			name: "legacy bypass mode migrates to all",
			settings: smart.RoutingSettings{
				Mode: smart.ModeBypass,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldDisconnectForRoutingSettings(test.settings); got != test.disconnect {
				t.Fatalf("shouldDisconnectForRoutingSettings() = %v, want %v", got, test.disconnect)
			}
		})
	}
}

func TestServiceSectionCopyMatchesRoutingMode(t *testing.T) {
	tests := []struct {
		mode  smart.Mode
		title string
	}{
		{mode: smart.ModeAll, title: "Сервисы"},
		{mode: smart.ModeSelected, title: "Сервисы через VPN"},
		{mode: smart.ModeBypass, title: "Сервисы"},
	}
	for _, test := range tests {
		title, detail := serviceSectionCopy(test.mode)
		if title != test.title || detail == "" {
			t.Fatalf("serviceSectionCopy(%q) = %q, %q", test.mode, title, detail)
		}
	}
}

func TestServiceSelectionEnabledOnlyForSelectedMode(t *testing.T) {
	if serviceSelectionEnabled(smart.ModeAll) || serviceSelectionEnabled(smart.ModeBypass) {
		t.Fatal("service switches are enabled outside selected-only mode")
	}
	if !serviceSelectionEnabled(smart.ModeSelected) {
		t.Fatal("service switches are disabled in selected-only mode")
	}
}

func TestRoutingSummaryShowsAllModeRules(t *testing.T) {
	settings := smart.DefaultSettings()
	settings.CustomRules = []smart.CustomRule{{Enabled: true, Target: smart.TargetDirect}}
	summary := routingSummaryCopy(settings)
	if !strings.Contains(summary, "активных правил: 1") {
		t.Fatalf("unexpected all-mode summary: %q", summary)
	}
}

func TestMixColorKeepsEndpoints(t *testing.T) {
	from := walk.RGB(10, 20, 30)
	to := walk.RGB(110, 120, 130)
	if got := mixColor(from, to, 0); got != from {
		t.Fatalf("mix start = %v, want %v", got, from)
	}
	if got := mixColor(from, to, 1); got != to {
		t.Fatalf("mix end = %v, want %v", got, to)
	}
}

func TestMoveProfileSettingsPreservesRoutingConfiguration(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	settings := smart.DefaultSettings()
	settings.Mode = smart.ModeSelected
	settings.SelectedApps = []string{"discord"}
	settings.CustomRules = []smart.CustomRule{{
		ID:      "rule-browser",
		Name:    "Browser",
		Kind:    smart.RuleApplication,
		Value:   "browser.exe",
		Target:  smart.TargetVPN,
		Enabled: true,
	}}
	if err := smart.SaveSettings("Старый профиль", settings); err != nil {
		t.Fatal(err)
	}
	if err := moveProfileSettings("Старый профиль", "Новый профиль", settings); err != nil {
		t.Fatal(err)
	}

	loaded, err := smart.LoadSettings("Новый профиль")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != smart.ModeSelected || len(loaded.SelectedApps) != 1 || loaded.SelectedApps[0] != "discord" || len(loaded.CustomRules) != 1 {
		t.Fatalf("moved settings changed: %#v", loaded)
	}
	old, err := smart.LoadSettings("Старый профиль")
	if err != nil {
		t.Fatal(err)
	}
	if old.Mode != smart.ModeAll || len(old.CustomRules) != 0 {
		t.Fatalf("old settings were not removed: %#v", old)
	}
}

func TestReadProfileFilesReadsConfFromZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	profile, err := archive.Create("nested/Phone.conf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = profile.Write([]byte("[Interface]\nAddress = 10.0.0.2/32\n")); err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	profiles, err := readProfileFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Name != "Phone" || !strings.Contains(profiles[0].Config, "[Interface]") {
		t.Fatalf("unexpected profiles: %#v", profiles)
	}
}

func TestReadProfileFilesRejectsOversizedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.conf")
	if err := os.WriteFile(path, make([]byte, maxImportedProfileSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	if profiles, err := readProfileFiles([]string{path}); err == nil || len(profiles) != 0 {
		t.Fatalf("oversized profile accepted: count=%d err=%v", len(profiles), err)
	}
}

func TestReadProfileFilesCapsArchiveCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for i := 0; i < maxImportedProfileCount+5; i++ {
		entry, createErr := archive.Create(filepath.ToSlash(filepath.Join("profiles", strings.Repeat("x", i%20+1)+string(rune('A'+i%26))+".conf")))
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write([]byte("[Interface]\n")); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	profiles, err := readProfileFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != maxImportedProfileCount {
		t.Fatalf("profile count = %d, want %d", len(profiles), maxImportedProfileCount)
	}
}
