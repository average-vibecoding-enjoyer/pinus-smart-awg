/* SPDX-License-Identifier: MIT */

package ui

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const pinusVPNBotURL = "https://t.me/pinusvpn_bot"

func openPinusVPNBot(owner walk.Form) {
	win.ShellExecute(owner.Handle(), nil, windows.StringToUTF16Ptr(pinusVPNBotURL), nil, nil, win.SW_SHOWNORMAL)
}
