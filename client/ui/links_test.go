/* SPDX-License-Identifier: MIT */

package ui

import (
	"net/url"
	"testing"
)

func TestPinusVPNBotURL(t *testing.T) {
	parsed, err := url.ParseRequestURI(pinusVPNBotURL)
	if err != nil {
		t.Fatalf("invalid bot URL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "t.me" || parsed.Path != "/pinusvpn_bot" {
		t.Fatalf("unexpected bot URL: %s", pinusVPNBotURL)
	}
}
