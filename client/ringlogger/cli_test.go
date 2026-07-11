/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2022 WireGuard LLC. All Rights Reserved.
 */

package ringlogger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ringlogger_test.bin")
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		rl, err := NewRinglogger(path, "ONE")
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 1024; i++ {
			fmt.Fprintf(rl, "bla bla bla %d", i)
		}
		rl.Close()
		wg.Done()
	}()
	go func() {
		rl, err := NewRinglogger(path, "TWO")
		if err != nil {
			t.Fatal(err)
		}
		for i := 1024; i < 2047; i++ {
			fmt.Fprintf(rl, "bla bla bla %d", i)
		}
		rl.Close()
		wg.Done()
	}()
	wg.Wait()
}

func TestWriteText(t *testing.T) {
	if os.Getenv("PINUS_RUN_RINGLOGGER_CLI_TESTS") != "1" {
		t.Skip("set PINUS_RUN_RINGLOGGER_CLI_TESTS=1 and pass a text argument")
	}
	rl, err := NewRinglogger("ringlogger_test.bin", "TXT")
	if err != nil {
		t.Fatal(err)
	}
	if len(os.Args) != 3 {
		t.Fatal("Should pass exactly one argument")
	}
	fmt.Fprint(rl, os.Args[2])
	rl.Close()
}

func TestDump(t *testing.T) {
	if os.Getenv("PINUS_RUN_RINGLOGGER_CLI_TESTS") != "1" {
		t.Skip("set PINUS_RUN_RINGLOGGER_CLI_TESTS=1 to run CLI log dumping")
	}
	rl, err := NewRinglogger("ringlogger_test.bin", "DMP")
	if err != nil {
		t.Fatal(err)
	}
	_, err = rl.WriteTo(os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	rl.Close()
}

func TestFollow(t *testing.T) {
	if os.Getenv("PINUS_RUN_RINGLOGGER_CLI_TESTS") != "1" {
		t.Skip("set PINUS_RUN_RINGLOGGER_CLI_TESTS=1 to run the continuous log follower")
	}
	rl, err := NewRinglogger("ringlogger_test.bin", "FOL")
	if err != nil {
		t.Fatal(err)
	}
	cursor := CursorAll
	for {
		var lines []FollowLine
		lines, cursor = rl.FollowFromCursor(cursor)
		for _, line := range lines {
			fmt.Printf("%v: %s\n", line.Stamp, line.Line)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
