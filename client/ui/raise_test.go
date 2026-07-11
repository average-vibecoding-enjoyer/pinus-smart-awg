/* SPDX-License-Identifier: MIT */

package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecutablesMatchByContents(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.exe")
	second := filepath.Join(directory, "second.exe")
	third := filepath.Join(directory, "third.exe")
	if err := os.WriteFile(first, []byte("same executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("same executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(third, []byte("different executable"), 0600); err != nil {
		t.Fatal(err)
	}

	match, err := executablesMatch(first, second)
	if err != nil {
		t.Fatal(err)
	}
	if !match {
		t.Fatal("identical executables were treated as different")
	}
	match, err = executablesMatch(first, third)
	if err != nil {
		t.Fatal(err)
	}
	if match {
		t.Fatal("different executables were treated as identical")
	}
}
