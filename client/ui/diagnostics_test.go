/* SPDX-License-Identifier: MIT */

package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

func TestNetworkSupportTextExplainsLeakProtection(t *testing.T) {
	if text := networkSupportText(smart.ProfileNetworkSupport{IPv4: true}); !strings.Contains(text, "IPv6 закрыт") {
		t.Fatalf("unexpected IPv4-only text: %q", text)
	}
	if text := networkSupportText(smart.ProfileNetworkSupport{IPv4: true, IPv6: true}); text != "IPv4 + IPv6" {
		t.Fatalf("unexpected dual-stack text: %q", text)
	}
}

func TestResolveEndpointCheckDoesNotQueryLiteralIP(t *testing.T) {
	config := &conf.Config{Peers: []conf.Peer{{Endpoint: conf.Endpoint{Host: "192.0.2.10", Port: 443}}}}
	check := resolveEndpointCheck(context.Background(), config)
	if check.Level != diagnosticGood || !strings.Contains(check.Detail, "IP-адресом") {
		t.Fatalf("unexpected endpoint check: %#v", check)
	}
}

func TestDiagnosticReportStringIsReadable(t *testing.T) {
	report := diagnosticReport{
		GeneratedAt: time.Date(2026, 7, 11, 15, 0, 0, 0, time.UTC),
		Checks: []diagnosticCheck{
			{Name: "Engine SHA", Detail: "проверен", Level: diagnosticGood},
			{Name: "DNS endpoint", Detail: "timeout", Level: diagnosticFailure},
		},
	}
	text := report.String()
	if !strings.Contains(text, "Engine SHA: [OK]") || !strings.Contains(text, "DNS endpoint: [FAIL]") {
		t.Fatalf("unexpected report text: %q", text)
	}
}
