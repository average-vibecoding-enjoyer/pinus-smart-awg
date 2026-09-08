/* SPDX-License-Identifier: MIT */

package smart

import (
	"testing"
)

func TestAIServiceIncludesMajorProviders(t *testing.T) {
	var ai *Service
	for _, service := range ServiceCatalog() {
		if service.ID == AIServiceID {
			copy := service
			ai = &copy
			break
		}
	}
	if ai == nil {
		t.Fatal("AI service is missing")
	}
	required := []string{
		"chatgpt.com", "claude.ai", "gemini.google.com", "grok.com",
		"perplexity.ai", "deepseek.com", "copilot.microsoft.com", "mistral.ai",
	}
	for _, domain := range required {
		found := false
		for _, value := range ai.Domains {
			if value == domain {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AI domain %q is missing", domain)
		}
	}
}

func TestVersionThreeDefaultSelectionEnablesAIOnUpgrade(t *testing.T) {
	settings := RoutingSettings{
		Version:      3,
		Mode:         ModeSelected,
		SelectedApps: append([]string(nil), legacyDefaultServiceIDs...),
	}
	normalized, err := settings.Normalized()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Version != settingsVersion {
		t.Fatalf("settings version = %d, want %d", normalized.Version, settingsVersion)
	}
	selectedAI := false
	for _, id := range normalized.SelectedApps {
		selectedAI = selectedAI || id == AIServiceID
	}
	if !selectedAI {
		t.Fatal("AI service was not enabled for the untouched legacy selection")
	}
}

func TestVersionThreeCustomSelectionDoesNotEnableAIOnUpgrade(t *testing.T) {
	settings := RoutingSettings{
		Version:      3,
		Mode:         ModeSelected,
		SelectedApps: []string{"discord", "telegram"},
	}
	normalized, err := settings.Normalized()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range normalized.SelectedApps {
		if id == AIServiceID {
			t.Fatal("AI service was silently enabled for a customized selection")
		}
	}
}

func TestAIServiceBuildsProcessAndDomainRoutes(t *testing.T) {
	settings := DefaultSettings()
	settings.Mode = ModeSelected
	settings.SelectedApps = []string{AIServiceID}
	data, err := BuildConfig(parseTestProfile(t), settings)
	if err != nil {
		t.Fatal(err)
	}
	rules := routeRules(t, decodeRoute(t, data))
	processRule := findRuleContaining(t, rules, "process_name", "ChatGPT.exe")
	domainRule := findRuleContaining(t, rules, "domain", "chatgpt.com")
	processes, _ := processRule["process_name"].([]any)
	domains, _ := domainRule["domain"].([]any)
	if !anyString(processes, "ChatGPT.exe") {
		t.Fatalf("ChatGPT process is missing: %#v", processes)
	}
	for _, domain := range []string{"chatgpt.com", "claude.ai", "gemini.google.com", "copilot.microsoft.com"} {
		if !anyString(domains, domain) {
			t.Errorf("AI route is missing %q", domain)
		}
	}
}

func anyString(values []any, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
