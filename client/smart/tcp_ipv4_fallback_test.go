package smart

import (
	"strings"
	"testing"
)

func TestTCPIPv4FallbackUsesExistingVPNDNS(t *testing.T) {
	for _, test := range []struct{ name, profile, want string }{
		{"IPv4", testProfile, "profile-dns-0"},
		{"dual-stack", dualStackTestProfile, "profile-dns-0"},
		{"IPv6-DNS-first", strings.Replace(dualStackTestProfile, "DNS = 1.1.1.1, 2606:4700:4700::1111", "DNS = 2606:4700:4700::1111, 1.1.1.1", 1), "profile-dns-1"},
		{"only-IPv6-DNS", strings.Replace(dualStackTestProfile, "DNS = 1.1.1.1, 2606:4700:4700::1111", "DNS = 2606:4700:4700::1111", 1), "profile-dns-0"},
		{"IPv6-only-profile", strings.Replace(dualStackTestProfile, "Address = 10.77.77.2/32, fd42:42:42::2/128", "Address = fd42:42:42::2/128", 1), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []Mode{ModeAll, ModeSelected} {
				settings := DefaultSettings()
				settings.Mode = mode
				settings = settings.WithServiceVPN(VKServiceID, false)
				data, err := BuildConfig(parseProfile(t, test.profile), settings)
				if err != nil {
					t.Fatal(err)
				}
				root := decodeRoot(t, data)
				endpoint := root["endpoints"].([]any)[0].(map[string]any)
				resolver, _ := endpoint["tcp_ipv4_fallback_resolver"].(string)
				if resolver != test.want {
					t.Fatalf("mode %s resolver=%q want %q", mode, resolver, test.want)
				}
				if resolver == "" {
					continue
				}
				found := false
				for _, serverValue := range root["dns"].(map[string]any)["servers"].([]any) {
					server := serverValue.(map[string]any)
					if server["tag"] == resolver {
						found = true
						if server["detour"] != "awg-out" {
							t.Fatalf("fallback resolver escaped AWG: %#v", server)
						}
					}
				}
				if !found {
					t.Fatal("fallback DNS tag is missing")
				}
			}
		})
	}
}
