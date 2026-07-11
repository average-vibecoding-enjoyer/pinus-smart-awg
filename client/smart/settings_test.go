/* SPDX-License-Identifier: MIT */

package smart

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

const testProfile = `[Interface]
PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=
Address = 10.77.77.2/32
DNS = 1.1.1.1
Jc = 6
Jmin = 40
Jmax = 70
S1 = 657
S2 = 1002
S4 = 8
H1 = 2881071079
H2 = 4232111507
H3 = 2348639460
H4 = 96722292
I1 = 0000000000000000000000000000000000000000000000000000000000000000

[Peer]
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
Endpoint = 198.51.100.10:443
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

func parseTestProfile(t *testing.T) *conf.Config {
	t.Helper()
	config, err := conf.FromWgQuick(testProfile, "test")
	if err != nil {
		t.Fatalf("parse profile: %v", err)
	}
	return config
}

func decodeRoute(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("decode generated config: %v", err)
	}
	route, ok := root["route"].(map[string]any)
	if !ok {
		t.Fatal("generated config has no route object")
	}
	return route
}

func decodeRoot(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("decode generated config: %v", err)
	}
	return root
}

func routeRules(t *testing.T, route map[string]any) []any {
	t.Helper()
	rules, ok := route["rules"].([]any)
	if !ok {
		t.Fatal("generated route has no rules")
	}
	return rules
}

func findRule(t *testing.T, rules []any, field string) map[string]any {
	t.Helper()
	for _, value := range rules {
		rule, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := rule[field]; exists {
			return rule
		}
	}
	t.Fatalf("no rule with field %q", field)
	return nil
}

func TestParseLegacySocialMode(t *testing.T) {
	settings, err := ParseSettingsPayload("social")
	if err != nil {
		t.Fatal(err)
	}
	if settings.Mode != ModeSelected {
		t.Fatalf("mode = %q, want %q", settings.Mode, ModeSelected)
	}
	if len(settings.SelectedApps) != len(ServiceCatalog()) {
		t.Fatalf("selected apps = %d, want %d", len(settings.SelectedApps), len(ServiceCatalog()))
	}
}

func TestModeCatalogExposesOnlyAllAndSelected(t *testing.T) {
	labels := ModeLabels()
	if len(labels) != 2 || ModeFromIndex(0) != ModeAll || ModeFromIndex(1) != ModeSelected {
		t.Fatalf("unexpected routing mode catalog: labels=%#v values=%q,%q", labels, ModeFromIndex(0), ModeFromIndex(1))
	}
	if NormalizeMode(string(ModeBypass)) != ModeAll || IndexOfMode(ModeBypass) != 0 {
		t.Fatal("legacy bypass mode was not migrated to all-traffic mode")
	}
}

func TestSettingsNormalizeCustomRules(t *testing.T) {
	settings := DefaultSettings()
	settings.CustomRules = []CustomRule{
		{Kind: RuleApplication, Value: "custom-app", Target: TargetVPN, Enabled: true},
		{Kind: RuleDomain, Value: "https://Example.COM/watch", Target: TargetDirect, Enabled: true},
		{Kind: RuleCIDR, Value: "203.0.113.7", Target: TargetVPN, Enabled: true},
	}
	normalized, err := settings.Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.CustomRules[0].Value != "custom-app.exe" {
		t.Fatalf("application = %q", normalized.CustomRules[0].Value)
	}
	if normalized.CustomRules[1].Value != "example.com" {
		t.Fatalf("domain = %q", normalized.CustomRules[1].Value)
	}
	if normalized.CustomRules[2].Value != "203.0.113.7/32" {
		t.Fatalf("cidr = %q", normalized.CustomRules[2].Value)
	}
}

func TestHasEnabledCustomRules(t *testing.T) {
	settings := DefaultSettings()
	settings.CustomRules = []CustomRule{{Enabled: false}}
	if HasEnabledCustomRules(settings) {
		t.Fatal("disabled rule was treated as active")
	}
	settings.CustomRules = append(settings.CustomRules, CustomRule{Enabled: true})
	if !HasEnabledCustomRules(settings) {
		t.Fatal("enabled rule was not detected")
	}
}

func TestNormalizeDomainUsesURLAndIDNParsing(t *testing.T) {
	domain, err := normalizeDomain("https://*.Пример.РФ:443/watch?q=1")
	if err != nil {
		t.Fatal(err)
	}
	if domain != "xn--e1afmkfd.xn--p1ai" {
		t.Fatalf("domain = %q", domain)
	}
	if _, err := normalizeDomain("https://1.1.1.1/path"); err == nil {
		t.Fatal("IP address was accepted as a domain rule")
	}
}

func TestNormalizeApplicationRejectsNonExecutablePath(t *testing.T) {
	if _, err := normalizeApplication(`C:\Program Files\Example\readme.txt`); err == nil {
		t.Fatal("non-executable path was accepted as an application rule")
	}
}

func TestSafeTunnelFileStemIsStableAndCollisionResistant(t *testing.T) {
	first := SafeTunnelFileStem("Телефон")
	if first != SafeTunnelFileStem("Телефон") {
		t.Fatal("safe tunnel filename is not stable")
	}
	second := SafeTunnelFileStem("Планшет")
	if first == second {
		t.Fatalf("different Unicode profile names collided: %q", first)
	}
	if strings.ContainsAny(first, `\\/:*?\"<>|`) {
		t.Fatalf("unsafe filename characters in %q", first)
	}
}

func TestBuildConfigSelectedMode(t *testing.T) {
	settings := DefaultSettings()
	settings.Mode = ModeSelected
	settings.SelectedApps = []string{"discord"}
	data, err := BuildConfig(parseTestProfile(t), settings)
	if err != nil {
		t.Fatal(err)
	}
	route := decodeRoute(t, data)
	if route["final"] != "direct" {
		t.Fatalf("final = %v, want direct", route["final"])
	}
	processRule := findRule(t, routeRules(t, route), "process_name")
	if processRule["outbound"] != "awg-out" {
		t.Fatalf("process outbound = %v", processRule["outbound"])
	}
	root := decodeRoot(t, data)
	endpoint := root["endpoints"].([]any)[0].(map[string]any)
	if endpoint["s4"] != float64(8) || endpoint["i1"] != "0000000000000000000000000000000000000000000000000000000000000000" {
		t.Fatalf("AWG obfuscation parameters were not preserved: s4=%v i1=%v", endpoint["s4"], endpoint["i1"])
	}
	dns := root["dns"].(map[string]any)
	if dns["strategy"] != "ipv4_only" {
		t.Fatalf("DNS strategy = %v, want ipv4_only", dns["strategy"])
	}
	server := dns["servers"].([]any)[0].(map[string]any)
	if server["detour"] != "awg-out" {
		t.Fatalf("selected-mode DNS detour = %v, want awg-out", server["detour"])
	}
}

func TestCustomRulePrecedesServiceAndDefaultRoutes(t *testing.T) {
	settings := DefaultSettings()
	settings.Mode = ModeSelected
	settings.SelectedApps = []string{"discord"}
	settings.CustomRules = []CustomRule{
		{ID: "discord-direct", Name: "Discord direct", Kind: RuleApplication, Value: "Discord.exe", Target: TargetDirect, Enabled: true},
	}
	data, err := BuildConfig(parseTestProfile(t), settings)
	if err != nil {
		t.Fatal(err)
	}
	rules := routeRules(t, decodeRoute(t, data))
	customIndex := -1
	serviceIndex := -1
	for i, value := range rules {
		rule, ok := value.(map[string]any)
		if !ok {
			continue
		}
		processes, _ := rule["process_name"].([]any)
		if len(processes) == 1 && processes[0] == "Discord.exe" && rule["outbound"] == "direct" {
			customIndex = i
		}
		if len(processes) > 1 && rule["outbound"] == "awg-out" {
			serviceIndex = i
		}
	}
	if customIndex < 0 || serviceIndex < 0 || customIndex >= serviceIndex {
		t.Fatalf("custom rule must precede service rule: custom=%d service=%d", customIndex, serviceIndex)
	}
}

func TestServiceRuleIncludesExactSuffixRoots(t *testing.T) {
	settings := DefaultSettings()
	settings.Mode = ModeSelected
	settings.SelectedApps = []string{"youtube"}
	data, err := BuildConfig(parseTestProfile(t), settings)
	if err != nil {
		t.Fatal(err)
	}
	domainRule := findRule(t, routeRules(t, decodeRoute(t, data)), "domain")
	domains, _ := domainRule["domain"].([]any)
	found := false
	for _, domain := range domains {
		if domain == "youtubei.googleapis.com" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("exact API domain missing from service rule: %#v", domains)
	}
}

func TestBuildConfigAllModeAndTunCompatibility(t *testing.T) {
	settings := DefaultSettings()
	settings.CustomRules = []CustomRule{
		{
			ID:      "browser",
			Name:    "Browser",
			Kind:    RuleApplication,
			Value:   `C:\Program Files\Browser\browser.exe`,
			Target:  TargetDirect,
			Enabled: true,
		},
	}
	data, err := BuildConfig(parseTestProfile(t), settings)
	if err != nil {
		t.Fatal(err)
	}
	route := decodeRoute(t, data)
	if route["final"] != "awg-out" {
		t.Fatalf("final = %v, want awg-out", route["final"])
	}
	rules := routeRules(t, route)
	pathRule := findRule(t, rules, "process_path")
	if pathRule["outbound"] != "direct" {
		t.Fatalf("custom path outbound = %v", pathRule["outbound"])
	}
	root := decodeRoot(t, data)
	dns := root["dns"].(map[string]any)
	server := dns["servers"].([]any)[0].(map[string]any)
	if server["detour"] != "awg-out" {
		t.Fatalf("all-mode DNS detour = %v, want awg-out", server["detour"])
	}
	inbound := root["inbounds"].([]any)[0].(map[string]any)
	if inbound["interface_name"] != TunInterfaceName {
		t.Fatalf("TUN interface = %v, want %s", inbound["interface_name"], TunInterfaceName)
	}
	addresses := inbound["address"].([]any)
	if len(addresses) != 1 || addresses[0] != "172.19.77.1/30" {
		t.Fatalf("TUN addresses = %#v, want IPv4-only routing", addresses)
	}
	if inbound["stack"] != "mixed" {
		t.Fatalf("TUN stack = %v, want mixed", inbound["stack"])
	}
	if inbound["endpoint_independent_nat"] != true {
		t.Fatalf("endpoint-independent NAT = %v, want true", inbound["endpoint_independent_nat"])
	}
}

func TestSettingsPayloadRoundTrip(t *testing.T) {
	settings := DefaultSettings()
	settings.Mode = ModeSelected
	payload, err := SettingsPayload(settings)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSettingsPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Mode != settings.Mode || len(parsed.SelectedApps) != len(settings.SelectedApps) {
		t.Fatalf("round trip mismatch: %#v", parsed)
	}
}

func TestSettingsRemainSeparateForUnicodeProfiles(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	first := DefaultSettings()
	first.Mode = ModeSelected
	second := DefaultSettings()
	second.Mode = ModeAll
	if err := SaveSettings("Телефон", first); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings("Планшет", second); err != nil {
		t.Fatal(err)
	}
	loadedFirst, err := LoadSettings("Телефон")
	if err != nil {
		t.Fatal(err)
	}
	loadedSecond, err := LoadSettings("Планшет")
	if err != nil {
		t.Fatal(err)
	}
	if loadedFirst.Mode != ModeSelected || loadedSecond.Mode != ModeAll {
		t.Fatalf("profile settings collided: first=%q second=%q", loadedFirst.Mode, loadedSecond.Mode)
	}
	first.Mode = ModeAll
	if err := SaveSettings("Телефон", first); err != nil {
		t.Fatal(err)
	}
	loadedFirst, err = LoadSettings("Телефон")
	if err != nil || loadedFirst.Mode != ModeAll {
		t.Fatalf("atomic settings replacement failed: mode=%q err=%v", loadedFirst.Mode, err)
	}
}

func TestBundledEngineIntegrity(t *testing.T) {
	enginePath := filepath.Clean(filepath.Join("..", "..", "engine", "amnezia-box.exe"))
	if _, err := os.Stat(enginePath); err != nil {
		t.Skip("bundled engine is not available")
	}
	if err := VerifyEngine(enginePath); err != nil {
		t.Fatal(err)
	}
	modified := filepath.Join(t.TempDir(), "amnezia-box.exe")
	if err := os.WriteFile(modified, []byte("not the bundled engine"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyEngine(modified); err == nil {
		t.Fatal("modified engine passed integrity verification")
	}
}

func TestGeneratedConfigsPassBundledEngineCheck(t *testing.T) {
	enginePath := filepath.Clean(filepath.Join("..", "..", "engine", "amnezia-box.exe"))
	if _, err := os.Stat(enginePath); err != nil {
		t.Skip("bundled engine is not available")
	}

	cases := []struct {
		name     string
		settings RoutingSettings
	}{
		{name: "all", settings: DefaultSettings()},
		{name: "selected", settings: func() RoutingSettings {
			settings := DefaultSettings()
			settings.Mode = ModeSelected
			return settings
		}()},
		{name: "all with custom rules", settings: func() RoutingSettings {
			settings := DefaultSettings()
			settings.CustomRules = []CustomRule{
				{ID: "app", Name: "App", Kind: RuleApplication, Value: "example.exe", Target: TargetVPN, Enabled: true},
				{ID: "domain", Name: "Domain", Kind: RuleDomain, Value: "example.com", Target: TargetDirect, Enabled: true},
				{ID: "network", Name: "Network", Kind: RuleCIDR, Value: "203.0.113.0/24", Target: TargetVPN, Enabled: true},
			}
			return settings
		}()},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data, err := BuildConfig(parseTestProfile(t), test.settings)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(enginePath, "check", "-c", path, "--disable-color")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("engine rejected config: %v\n%s", err, output)
			}
		})
	}
}
