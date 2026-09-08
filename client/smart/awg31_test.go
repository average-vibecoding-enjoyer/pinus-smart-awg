package smart

import (
	"github.com/amnezia-vpn/amneziawg-windows/conf"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAWG31EndpointMapping(t *testing.T) {
	extra := `S1 = 16
S2 = 16
S3 = 16
S4 = 16
HeaderProtectionKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=
ContentPaddingAddition = 0-32
RekeyAfterTime = 110-130
RekeyTimeout = 4-6
RejectAfterTime = 170-190
KeepaliveTimeout = 8-12
MaxHandshakeAttempts = 15-20
RandomTrailers = on
DisableCookies = off
`
	profile := strings.Replace(dualStackTestProfile, "[Peer]", extra+"\n[Peer]", 1)
	profile = strings.Replace(profile, "PersistentKeepalive = 25", "PersistentKeepalive = 20-30", 1)
	config, err := conf.FromWgQuick(profile, "Synthetic31")
	if err != nil {
		t.Fatal(err)
	}
	data, err := BuildConfig(config, DefaultSettings().WithServiceVPN(VKServiceID, false))
	if err != nil {
		t.Fatal(err)
	}
	ep := decodeRoot(t, data)["endpoints"].([]any)[0].(map[string]any)
	for key, want := range map[string]any{"header_protection_key": config.Interface.HeaderProtectionKey.String(), "content_padding_addition": "0-32", "rekey_after_time": "110-130", "rekey_timeout": "4-6", "reject_after_time": "170-190", "keepalive_timeout": "8-12", "max_handshake_attempts": "15-20", "random_trailers": true, "disable_cookies": false} {
		if ep[key] != want {
			t.Errorf("lost AWG setting %s", key)
		}
	}
	if ep["peers"].([]any)[0].(map[string]any)["persistent_keepalive_interval"] != "20-30" {
		t.Fatal("lost keepalive range")
	}
	if dir := os.Getenv("PINUS_FIXTURE_DIR"); dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "awg31.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
