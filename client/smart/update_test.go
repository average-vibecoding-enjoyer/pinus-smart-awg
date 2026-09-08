package smart

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestRegionalDefaultsAreIndependentByMode(t *testing.T) {
	s := DefaultSettings()
	for _, id := range []string{VKServiceID, RussianServiceID} {
		if !s.ServiceUsesVPN(id) {
			t.Fatal("full mode must default to VPN")
		}
	}
	s = s.WithServiceVPN(VKServiceID, false)
	selected := s
	selected.Mode = ModeSelected
	for _, id := range []string{VKServiceID, RussianServiceID} {
		if selected.ServiceUsesVPN(id) {
			t.Fatal("selected mode must default to direct")
		}
	}
	selected = selected.WithServiceVPN(VKServiceID, true)
	if s.ServiceUsesVPN(VKServiceID) {
		t.Fatal("settings copies share mutable service routes")
	}
	selected.Mode = ModeAll
	if selected.ServiceUsesVPN(VKServiceID) {
		t.Fatal("switching modes overwrote explicit full-mode choice")
	}
	encoded, err := SettingsPayload(selected)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseSettingsPayload(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.ServiceRoutes, selected.ServiceRoutes) {
		t.Fatal("per-mode settings lost in IPC")
	}
}

func TestVKRouteHasIndependentPriorityAndCoversPhotoDomain(t *testing.T) {
	settings := DefaultSettings().WithServiceVPN(RussianServiceID, false)
	for _, domain := range []string{"vk.ru", "api.vk.com", "sun1-23.vkuserphoto.ru", "calls.okcdn.ru", "music.vk.ru", "vkvideo.ru"} {
		got, err := ExplainRoute(settings, RouteQuery{Domain: domain})
		if err != nil {
			t.Fatal(err)
		}
		if got.Target != TargetVPN || got.Rule != "VK" {
			t.Fatalf("%s: %+v", domain, got)
		}
	}
	got, err := ExplainRoute(settings, RouteQuery{Domain: "online.sberbank.ru"})
	if err != nil || got.Target != TargetDirect {
		t.Fatalf("bank: %+v %v", got, err)
	}
	got, err = ExplainRoute(settings, RouteQuery{Domain: "notvk.ru"})
	if err != nil || got.Rule == "VK" {
		t.Fatalf("suffix boundary: %+v %v", got, err)
	}
}

func TestExplicitPrivateRulePrecedesLANBypass(t *testing.T) {
	settings := DefaultSettings()
	settings.CustomRules = []CustomRule{{ID: "private-vpn", Name: "Private VPN", Kind: RuleCIDR, Value: "10.50.0.0/16", Target: TargetVPN, Enabled: true}}
	result, err := ExplainRoute(settings, RouteQuery{IP: "10.50.1.1"})
	if err != nil || result.Target != TargetVPN {
		t.Fatalf("%+v %v", result, err)
	}
	data, err := BuildConfig(parseTestProfile(t), settings)
	if err != nil {
		t.Fatal(err)
	}
	explicit := bytes.Index(data, []byte("10.50.0.0/16"))
	lan := bytes.Index(data, []byte("10.0.0.0/8"))
	if explicit < 0 || lan < 0 || explicit >= lan {
		t.Fatal("compiler priority differs from explanation")
	}
}

func TestBackupRoundTripRejectsWrongPasswordAndTampering(t *testing.T) {
	original := Backup{Version: 1, CreatedAt: time.Now().UTC(), Profiles: []BackupProfile{{Name: "synthetic", Config: testProfile, Settings: DefaultSettings().WithServiceVPN(VKServiceID, false)}}}
	encrypted, err := EncryptBackup(original, "synthetic long password")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("PrivateKey")) {
		t.Fatal("plaintext leaked into backup")
	}
	decoded, err := DecryptBackup(encrypted, "synthetic long password")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatal("backup does not preserve configuration and rules")
	}
	if _, err = DecryptBackup(encrypted, "incorrect"); err == nil {
		t.Fatal("wrong password accepted")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, err = DecryptBackup(encrypted, "synthetic long password"); err == nil {
		t.Fatal("tampered backup accepted")
	}
}

func TestUntrustedCatalogCannotEnterSettingsSnapshot(t *testing.T) {
	s := DefaultSettings()
	s.CatalogEnvelope = json.RawMessage(`{"schema":1,"key_id":"434f57314cd1ee6c","payload":"e30=","signature":""}`)
	if _, err := s.Normalized(); err == nil {
		t.Fatal("unsigned catalog accepted")
	}
}

func TestRegionalRulesDoNotBypassBrowserOrWholeCountry(t *testing.T) {
	for _, service := range RegionalServices() {
		for _, process := range service.ProcessNames {
			if process == "chrome.exe" || process == "msedge.exe" || process == "firefox.exe" {
				t.Fatal("whole browser included")
			}
		}
		for _, suffix := range service.DomainSuffixes {
			if suffix == ".ru" || suffix == ".com" || suffix == ".net" {
				t.Fatal("whole TLD included")
			}
		}
	}
}

func findRuleContaining(t *testing.T, rules []any, key, wanted string) map[string]any {
	t.Helper()
	for _, raw := range rules {
		rule := raw.(map[string]any)
		values, _ := rule[key].([]any)
		if anyString(values, wanted) {
			return rule
		}
	}
	t.Fatalf("no rule for %s=%s", key, wanted)
	return nil
}
