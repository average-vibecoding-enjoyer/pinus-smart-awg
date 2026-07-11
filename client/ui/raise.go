/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2022 WireGuard LLC. All Rights Reserved.
 */

package ui

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"github.com/amnezia-vpn/amneziawg-windows-client/l18n"
)

func raise(hwnd win.HWND) {
	if win.IsIconic(hwnd) {
		win.ShowWindow(hwnd, win.SW_RESTORE)
	}

	win.SetActiveWindow(hwnd)
	win.SetWindowPos(hwnd, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_SHOWWINDOW)
	win.SetForegroundWindow(hwnd)
	win.SetWindowPos(hwnd, win.HWND_NOTOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_SHOWWINDOW)
}

func raiseRemote(hwnd win.HWND) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	win.SendMessage(hwnd, raiseMsg, 0, 0)
	currentForegroundHwnd := win.GetForegroundWindow()
	currentForegroundThreadId := win.GetWindowThreadProcessId(currentForegroundHwnd, nil)
	currentThreadId := win.GetCurrentThreadId()
	win.AttachThreadInput(int32(currentForegroundThreadId), int32(currentThreadId), true)
	win.SetWindowPos(hwnd, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_SHOWWINDOW)
	win.SetWindowPos(hwnd, win.HWND_NOTOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_SHOWWINDOW)
	win.SetForegroundWindow(hwnd)
	win.AttachThreadInput(int32(currentForegroundThreadId), int32(currentThreadId), false)
	win.SetFocus(hwnd)
	win.SetActiveWindow(hwnd)
}

func RaiseUI() bool {
	hwnd := win.FindWindow(windows.StringToUTF16Ptr(manageWindowWindowClass), nil)
	if hwnd == 0 {
		return false
	}
	if matches, err := runningUIExecutableMatches(hwnd); err == nil && !matches {
		return false
	}
	raiseRemote(hwnd)
	return true
}

func executablePathForWindow(hwnd win.HWND) (string, error) {
	var processID uint32
	win.GetWindowThreadProcessId(hwnd, &processID)
	if processID == 0 {
		return "", fmt.Errorf("window process is unavailable")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

func executableDigest(path string) ([sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return result, nil
}

func executablesMatch(left, right string) (bool, error) {
	if strings.EqualFold(left, right) {
		return true, nil
	}
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, err
	}
	if os.SameFile(leftInfo, rightInfo) {
		return true, nil
	}
	leftDigest, err := executableDigest(left)
	if err != nil {
		return false, err
	}
	rightDigest, err := executableDigest(right)
	if err != nil {
		return false, err
	}
	return leftDigest == rightDigest, nil
}

func runningUIExecutableMatches(hwnd win.HWND) (bool, error) {
	current, err := os.Executable()
	if err != nil {
		return false, err
	}
	running, err := executablePathForWindow(hwnd)
	if err != nil {
		return false, err
	}
	return executablesMatch(current, running)
}

func WaitForRaiseUIThenQuit() {
	var handle win.HWINEVENTHOOK
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	handle, err := win.SetWinEventHook(win.EVENT_OBJECT_CREATE, win.EVENT_OBJECT_CREATE, 0, func(hWinEventHook win.HWINEVENTHOOK, event uint32, hwnd win.HWND, idObject, idChild int32, idEventThread, dwmsEventTime uint32) uintptr {
		class := make([]uint16, len(manageWindowWindowClass)+2) /* Plus 2, one for the null terminator, and one to see if this is only a prefix */
		n, err := win.GetClassName(hwnd, &class[0], len(class))
		if err != nil || n != len(manageWindowWindowClass) || windows.UTF16ToString(class) != manageWindowWindowClass {
			return 0
		}
		win.UnhookWinEvent(handle)
		raiseRemote(hwnd)
		os.Exit(0)
		return 0
	}, 0, 0, win.WINEVENT_SKIPOWNPROCESS|win.WINEVENT_OUTOFCONTEXT)
	if err != nil {
		showErrorCustom(nil, "Pinus Smart AWG Detection Error", l18n.Sprintf("Unable to wait for Pinus Smart AWG window to appear: %v", err))
	}
	for {
		var msg win.MSG
		if m := win.GetMessage(&msg, 0, 0, 0); m != 0 {
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}
	}
}
