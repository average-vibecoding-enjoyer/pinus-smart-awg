/* SPDX-License-Identifier: MIT */

package manager

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
)

func TestRoutingBackendSelection(t *testing.T) {
	if !usesNativeTunnel(smart.RoutingSettings{Mode: smart.ModeAll}) {
		t.Fatal("all-traffic mode must use the native AWG tunnel")
	}
	if !usesNativeTunnel(smart.RoutingSettings{Mode: smart.ModeBypass}) {
		t.Fatal("legacy bypass mode must migrate to the native AWG tunnel")
	}
	if usesNativeTunnel(smart.RoutingSettings{Mode: smart.ModeSelected}) {
		t.Fatal("selected-only mode requires the smart routing engine")
	}
	allWithException := smart.RoutingSettings{
		Mode: smart.ModeAll,
		CustomRules: []smart.CustomRule{{
			Kind: smart.RuleDomain, Value: "example.com", Target: smart.TargetDirect, Enabled: true,
		}},
	}
	if usesNativeTunnel(allWithException) {
		t.Fatal("all-traffic mode with an enabled exception requires the smart routing engine")
	}
	allWithDisabledRule := allWithException
	allWithDisabledRule.CustomRules[0].Enabled = false
	if !usesNativeTunnel(allWithDisabledRule) {
		t.Fatal("disabled rules must not replace the native all-traffic tunnel")
	}
}

func TestWaitForSmartInterfaceReady(t *testing.T) {
	waitDone := make(chan struct{})
	calls := 0
	err := waitForSmartInterface(waitDone, func(name string) (*net.Interface, error) {
		if name != smart.TunInterfaceName {
			t.Fatalf("interface name = %q", name)
		}
		calls++
		if calls < 2 {
			return nil, errors.New("not ready")
		}
		return &net.Interface{Name: name, Flags: net.FlagUp}, nil
	}, nil, 250*time.Millisecond, 5*time.Millisecond, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("readiness probe calls = %d", calls)
	}
}

func TestWaitForSmartInterfaceDetectsProcessExit(t *testing.T) {
	waitDone := make(chan struct{})
	close(waitDone)
	err := waitForSmartInterface(waitDone, net.InterfaceByName, nil, time.Second, 5*time.Millisecond, 5*time.Millisecond)
	if !errors.Is(err, errSmartProcessExitedBeforeReady) {
		t.Fatalf("process exit error = %v", err)
	}
}

func TestWaitForSmartInterfaceTimesOut(t *testing.T) {
	waitDone := make(chan struct{})
	err := waitForSmartInterface(waitDone, func(string) (*net.Interface, error) {
		return nil, errors.New("not ready")
	}, nil, 30*time.Millisecond, 5*time.Millisecond, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), smart.TunInterfaceName) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestWaitForSmartInterfaceAcceptsEngineStartupLog(t *testing.T) {
	waitDone := make(chan struct{})
	err := waitForSmartInterface(waitDone, func(string) (*net.Interface, error) {
		return nil, errors.New("Windows interface lookup missed the adapter")
	}, func() bool {
		return true
	}, 250*time.Millisecond, 5*time.Millisecond, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSmartLogStartupProbeIgnoresPreviousRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.log")
	oldRun := []byte("INFO inbound/tun[tun-in]: started at " + smart.TunInterfaceName + "\n")
	if err := os.WriteFile(path, oldRun, 0600); err != nil {
		t.Fatal(err)
	}
	if smartLogHasStartup(path, int64(len(oldRun))) {
		t.Fatal("startup marker from an earlier run was accepted")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString("INFO sing-box started (0.23s)\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	if !smartLogHasStartup(path, int64(len(oldRun))) {
		t.Fatal("startup marker from the current run was not detected")
	}
}

func TestRotateSmartLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(smartLogMaxSize + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()

	if err := rotateSmartLog(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("current log still exists: %v", err)
	}
	if info, err := os.Stat(path + ".1"); err != nil || info.Size() != smartLogMaxSize+1 {
		t.Fatalf("rotated log mismatch: info=%v err=%v", info, err)
	}
}

func TestRotateSmartLogKeepsSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.log")
	if err := os.WriteFile(path, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := rotateSmartLog(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("unexpected rotated file: %v", err)
	}
}

func TestStopSmartProcessStateRemovesRuntimeConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	if err := os.WriteFile(path, []byte(`{"private_key":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stopSmartProcessState(&smartProcessState{configPath: path}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("runtime config still exists: %v", err)
	}
}

func TestStoppingPreviousSmartProcessKeepsReplacementRuntimeConfig(t *testing.T) {
	directory := t.TempDir()
	previousPath, err := reserveSmartRuntimeConfig(directory, "tunnel")
	if err != nil {
		t.Fatal(err)
	}
	replacementPath, err := reserveSmartRuntimeConfig(directory, "tunnel")
	if err != nil {
		t.Fatal(err)
	}
	if previousPath == replacementPath {
		t.Fatalf("runtime config paths must be generation-specific: %q", previousPath)
	}
	if err := os.WriteFile(previousPath, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacementPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := stopSmartProcessState(&smartProcessState{configPath: previousPath}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(previousPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous runtime config still exists: %v", err)
	}
	contents, err := os.ReadFile(replacementPath)
	if err != nil {
		t.Fatalf("replacement runtime config was removed: %v", err)
	}
	if string(contents) != "replacement" {
		t.Fatalf("replacement runtime config = %q", contents)
	}
}

func TestCleanupSmartConfigDirectoryRemovesOnlyRuntimeJSON(t *testing.T) {
	directory := t.TempDir()
	for name, contents := range map[string]string{
		"active.json":  "private key material",
		"UPPER.JSON":   "private key material",
		"session.log":  "diagnostics",
		"settings.txt": "keep",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(directory, "nested.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := cleanupSmartConfigDirectory(directory); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"active.json", "UPPER.JSON"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runtime config %q was not removed: %v", name, err)
		}
	}
	for _, name := range []string{"session.log", "settings.txt", "nested.json"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("unrelated path %q was removed: %v", name, err)
		}
	}
}

func TestCopyFileAtomicReplacesDestination(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.exe")
	destination := filepath.Join(directory, "destination.exe")
	if err := os.WriteFile(source, []byte("new binary"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old binary"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyFileAtomic(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new binary" {
		t.Fatalf("destination = %q", data)
	}
}
