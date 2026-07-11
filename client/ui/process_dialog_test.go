/* SPDX-License-Identifier: MIT */

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeRunningProcessesDeduplicatesExecutablePaths(t *testing.T) {
	values := mergeRunningProcesses([]runningProcess{
		{Name: "Discord.exe", Path: `C:\Apps\Discord.exe`},
		{Name: "discord.exe", Path: `c:\apps\DISCORD.exe`},
		{Name: "Discord.exe", Path: `D:\Portable\Discord.exe`},
		{Name: "Protected.exe"},
	})
	if len(values) != 3 {
		t.Fatalf("merged process count = %d, want 3: %#v", len(values), values)
	}
	for _, process := range values {
		if process.Path == `C:\Apps\Discord.exe` && process.Instances != 2 {
			t.Fatalf("instance count = %d, want 2", process.Instances)
		}
	}
}

func TestEnumerateRunningProcessesFindsCurrentTestProcess(t *testing.T) {
	processes, err := enumerateRunningProcesses()
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(executable)
	found := false
	for _, process := range processes {
		if strings.EqualFold(process.Name, name) {
			found = true
			if process.routeValue() == "" {
				t.Fatal("current process has no routing value")
			}
			break
		}
	}
	if !found {
		t.Fatalf("current test process %q was not enumerated", name)
	}
}

func TestFilterRunningProcessesUsesNameAndPath(t *testing.T) {
	values := []runningProcess{
		{Name: "Discord.exe", Path: `C:\Users\Example\Discord.exe`},
		{Name: "Telegram.exe", Path: `D:\telega\Telegram.exe`},
	}
	filtered := filterRunningProcesses(values, "telega d:")
	if len(filtered) != 1 || filtered[0].Name != "Telegram.exe" {
		t.Fatalf("unexpected filtered processes: %#v", filtered)
	}
	if filtered[0].routeValue() != `D:\telega\Telegram.exe` {
		t.Fatalf("route value = %q", filtered[0].routeValue())
	}
}
