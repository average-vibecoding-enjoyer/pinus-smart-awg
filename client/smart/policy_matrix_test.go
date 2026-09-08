package smart

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCompiledRegionalPolicyMatrix(t *testing.T) {
	for _, mode := range []Mode{ModeAll, ModeSelected} {
		for _, vk := range []bool{false, true} {
			for _, ru := range []bool{false, true} {
				name := fmt.Sprintf("%s-vk%t-ru%t", mode, vk, ru)
				t.Run(name, func(t *testing.T) {
					s := DefaultSettings()
					s.Mode = mode
					s = s.WithServiceVPN(VKServiceID, vk).WithServiceVPN(RussianServiceID, ru)
					data, err := BuildConfig(parseTestProfile(t), s)
					if err != nil {
						t.Fatal(err)
					}
					root := decodeRoot(t, data)
					route := root["route"].(map[string]any)
					dns := root["dns"].(map[string]any)
					direct := root["outbounds"].([]any)[0].(map[string]any)
					resolver, ok := direct["domain_resolver"].(map[string]any)
					if !ok || resolver["server"] != "direct-dns" || resolver["strategy"] != "prefer_ipv4" {
						t.Fatal("direct hostname connections inherit the VPN resolver")
					}
					if root["inbounds"].([]any)[0].(map[string]any)["strict_route"] != true {
						t.Fatal("direct exceptions must not disable DNS leak protection")
					}
					for _, item := range []struct {
						domain string
						vpn    bool
					}{{"vk.ru", vk}, {"api.mycdn.me", vk}, {"tbank.ru", ru}, {"yandex.ru", ru}} {
						expectedRoute, expectedDNS := "direct", "direct-dns"
						if item.vpn {
							expectedRoute = "awg-out"
							expectedDNS = "profile-dns-0"
						}
						rr := findRuleContaining(t, routeRules(t, route), "domain", item.domain)
						dr := findRuleContaining(t, dns["rules"].([]any), "domain", item.domain)
						if rr["outbound"] != expectedRoute || dr["server"] != expectedDNS {
							t.Fatalf("%s: route=%v dns=%v", item.domain, rr["outbound"], dr["server"])
						}
						explanation, err := ExplainRoute(s, RouteQuery{Domain: item.domain})
						if err != nil {
							t.Fatal(err)
						}
						if (explanation.Target == TargetVPN) != item.vpn {
							t.Fatal("explanation differs from compiler")
						}
					}
					if dir := os.Getenv("PINUS_FIXTURE_DIR"); dir != "" {
						// Explicitly requested, synthetic fixture output only.
						data, err = AddDiagnosticInbound(data, 19083, "synthetic-only-probe-password-12345678")
						if err != nil {
							t.Fatal(err)
						}
						if err = os.MkdirAll(dir, 0700); err != nil {
							t.Fatal(err)
						}
						if err = os.WriteFile(filepath.Join(dir, name+".json"), data, 0600); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}
