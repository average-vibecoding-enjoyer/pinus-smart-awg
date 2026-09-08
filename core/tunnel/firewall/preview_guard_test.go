package firewall

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestGuardCannotOwnForeignFilters(t *testing.T) {
	foreign := windows.GUID{Data1: 1}
	for _, filter := range []*wtFwpmFilter0{nil, {}, {providerKey: &foreign, subLayerKey: previewGuardSublayer}, {providerKey: &previewGuardProvider, subLayerKey: foreign}} {
		if guardOwned(filter) {
			t.Fatal("foreign filter treated as removable")
		}
	}
	if !guardOwned(&wtFwpmFilter0{providerKey: &previewGuardProvider, subLayerKey: previewGuardSublayer}) {
		t.Fatal("own filter not recognized")
	}
}
