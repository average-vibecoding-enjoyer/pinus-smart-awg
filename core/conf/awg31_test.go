package conf

import (
	"strings"
	"testing"
)

const awg31Synthetic = `[Interface]
PrivateKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=
Address = 10.77.0.2/32
MTU = 1280
S1 = 16
S2 = 16
S3 = 16
S4 = 16
H1 = 1
H2 = 2
H3 = 3
H4 = 4
HeaderProtectionKey = AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=
ContentPaddingAddition = 0-32
RekeyAfterTime = 110-130
RekeyTimeout = 4-6
RejectAfterTime = 170-190
KeepaliveTimeout = 8-12
MaxHandshakeAttempts = 15-20
RandomTrailers = on
DisableCookies = off
I1 = <b 0x1234><r 12>
[Peer]
PublicKey = AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=
Endpoint = [2001:db8::1]:443
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 20-30
`

func TestAWG31ConfigRoundTrip(t *testing.T) {
	c, err := FromWgQuick(awg31Synthetic, "Synthetic")
	if err != nil {
		t.Fatal(err)
	}
	again, err := FromWgQuick(c.ToWgQuick(), "Synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if again.ToWgQuick() != c.ToWgQuick() {
		t.Fatal("save/load changed configuration")
	}
	uapi, err := c.ToUAPI()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"header_protection_key=" + strings.Repeat("02", 32), "content_padding_addition=0-32", "rekey_after_time=110-130", "rekey_timeout=4-6", "reject_after_time=170-190", "keepalive_timeout=8-12", "max_handshake_attempts=15-20", "random_trailers=1", "disable_cookies=0", "persistent_keepalive_interval=20-30", "endpoint=[2001:db8::1]:443"} {
		if !strings.Contains(uapi, want+"\n") {
			t.Fatalf("missing UAPI setting %s", strings.Split(want, "=")[0])
		}
	}
	if c.ProtocolDescription() != "AWG 3.1" {
		t.Fatal("wrong protocol label")
	}
}

func TestAWGRejectsUnsafeValuesBeforeDeviceCreation(t *testing.T) {
	cases := [][2]string{
		{"S1 = 16", "S1 = 11"}, {"S4 = 16", "S4 = 65000"},
		{"H1 = 1", "H1 = 2-3"}, {"H1 = 1", "H1 = 4294967296"},
		{"RekeyTimeout = 4-6", "RekeyTimeout = 6-4"}, {"RandomTrailers = on", "RandomTrailers = maybe"},
		{"PersistentKeepalive = 20-30", "PersistentKeepalive = 30-20"},
		{"I1 = <b 0x1234><r 12>", "I1 = <r -1>"}, {"I1 = <b 0x1234><r 12>", "I1 = <dz -1>"},
		{"MTU = 1280", "MTU = 1280\nJmin = 40\nJmax = 20"},
	}
	for _, tc := range cases {
		t.Run(tc[1], func(t *testing.T) {
			if _, err := FromWgQuick(strings.Replace(awg31Synthetic, tc[0], tc[1], 1), "Synthetic"); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestLegacyConfigDoesNotAcquireAWG31Parameters(t *testing.T) {
	text := "[Interface]\nPrivateKey = AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=\nS1 = 22\nH1 = 4000000000-4000000010\n[Peer]\nPublicKey = AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=\nPersistentKeepalive = 25\n"
	c, err := FromWgQuick(text, "Old")
	if err != nil {
		t.Fatal(err)
	}
	quick := c.ToWgQuick()
	uapi, err := c.ToUAPI()
	if err != nil {
		t.Fatal(err)
	}
	for _, unexpected := range []string{"HeaderProtection", "RandomTrailers", "RekeyAfterTime", "header_protection", "random_trailers", "rekey_after_time"} {
		if strings.Contains(quick+uapi, unexpected) {
			t.Fatalf("legacy profile gained %s", unexpected)
		}
	}
	if !strings.Contains(quick, "PersistentKeepalive = 25") || !strings.Contains(quick, "H1 = 4000000000-4000000010") {
		t.Fatal("legacy values changed")
	}
}

func TestCPSGrammarBounds(t *testing.T) {
	for _, s := range []string{"<r -1>", "<rc -1>", "<rd -1>", "<dz -1>", "<r 65535>", "<r 40000><r 40000>", "<b 0x1>", "<t><t>", "<r 10", "abc", "<unknown>"} {
		if ValidateCPS(s) == nil {
			t.Errorf("accepted %q", s)
		}
	}
	for _, s := range []string{"<r 0>", "<b 0x0123><t><rc 50><rd 10>", "<d><ds><dz 2>", "<r 1000>"} {
		if err := ValidateCPS(s); err != nil {
			t.Errorf("rejected %q: %v", s, err)
		}
	}
}
