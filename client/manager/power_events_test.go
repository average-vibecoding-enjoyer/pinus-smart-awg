/* SPDX-License-Identifier: MIT */

package manager

import "testing"

func TestHandleSmartPowerEventPausesWithoutClearingDesiredIntent(t *testing.T) {
	paused := 0
	resumed := 0
	handled := handleSmartPowerEvent(
		pbtAPMSuspend,
		func(operation func()) { operation() },
		func() error { paused++; return nil },
		func() { resumed++ },
	)
	if !handled || paused != 1 || resumed != 0 {
		t.Fatalf("suspend dispatch: handled=%v paused=%d resumed=%d", handled, paused, resumed)
	}
}

func TestHandleSmartPowerEventSchedulesBothWindowsResumeVariants(t *testing.T) {
	for _, eventType := range []uint32{pbtAPMResumeSuspend, pbtAPMResumeAuto} {
		paused := 0
		resumed := 0
		handled := handleSmartPowerEvent(
			eventType,
			func(operation func()) { operation() },
			func() error { paused++; return nil },
			func() { resumed++ },
		)
		if !handled || paused != 0 || resumed != 1 {
			t.Fatalf("resume %#x dispatch: handled=%v paused=%d resumed=%d", eventType, handled, paused, resumed)
		}
	}
}

func TestHandleSmartPowerEventIgnoresUnknownEvent(t *testing.T) {
	if handleSmartPowerEvent(0xffff, func(operation func()) { operation() }, func() error { return nil }, func() {}) {
		t.Fatal("unknown power event was handled")
	}
}
