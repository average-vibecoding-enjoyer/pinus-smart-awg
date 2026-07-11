/* SPDX-License-Identifier: MIT */

package manager

import (
	"errors"
	"reflect"
	"testing"
)

func TestPlanNativeSwitchStopsEveryOtherProfile(t *testing.T) {
	plan := planNativeSwitch("Work", map[string]TunnelState{
		"Phone": TunnelStarted,
		"Home":  TunnelStarting,
		"Old":   TunnelStopped,
		"Work":  TunnelStopped,
	})
	if !plan.install || !reflect.DeepEqual(plan.stop, []string{"Home", "Phone"}) || !reflect.DeepEqual(plan.wait, []string{"Home", "Phone"}) {
		t.Fatalf("unexpected switch plan: %#v", plan)
	}
}

func TestPlanNativeSwitchKeepsAlreadyActiveTarget(t *testing.T) {
	plan := planNativeSwitch("Phone", map[string]TunnelState{
		"Phone": TunnelStarted,
		"Work":  TunnelUnknown,
	})
	if plan.install || !reflect.DeepEqual(plan.stop, []string{"Work"}) || !reflect.DeepEqual(plan.wait, []string{"Work"}) {
		t.Fatalf("unexpected active-target plan: %#v", plan)
	}
}

func TestPlanNativeSwitchWaitsForStoppingTargetBeforeInstall(t *testing.T) {
	plan := planNativeSwitch("Phone", map[string]TunnelState{"Phone": TunnelStopping})
	if !plan.install || len(plan.stop) != 0 || !reflect.DeepEqual(plan.wait, []string{"Phone"}) {
		t.Fatalf("unexpected stopping-target plan: %#v", plan)
	}
}

func TestExecuteNativeSwitchNeverInstallsBeforeEveryWait(t *testing.T) {
	var events []string
	plan := nativeSwitchPlan{stop: []string{"A", "B"}, wait: []string{"A", "B"}, install: true}
	err := executeNativeSwitch(plan,
		func(name string) error { events = append(events, "stop:"+name); return nil },
		func(name string) error { events = append(events, "wait:"+name); return nil },
		func() error { events = append(events, "install"); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"stop:A", "stop:B", "wait:A", "wait:B", "install"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestExecuteNativeSwitchDoesNotInstallAfterStopFailure(t *testing.T) {
	installed := false
	err := executeNativeSwitch(nativeSwitchPlan{stop: []string{"A"}, wait: []string{"A"}, install: true},
		func(string) error { return errors.New("injected stop failure") },
		func(string) error { return nil },
		func() error { installed = true; return nil },
	)
	if err == nil || installed {
		t.Fatalf("unsafe switch result: installed=%v err=%v", installed, err)
	}
}
