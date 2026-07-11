/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2022 WireGuard LLC. All Rights Reserved.
 */

package ui

import (
	"runtime"
	"strings"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"github.com/amnezia-vpn/amneziawg-windows-client/l18n"
	"github.com/amnezia-vpn/amneziawg-windows-client/version"
)

var (
	easterEggIndex     = -1
	showingAboutDialog *walk.Dialog
)

func onAbout(owner walk.Form) {
	showError(runAboutDialog(owner), owner)
}

func runAboutDialog(owner walk.Form) error {
	if showingAboutDialog != nil {
		showingAboutDialog.Show()
		raise(showingAboutDialog.Handle())
		return nil
	}

	vbl := walk.NewVBoxLayout()
	vbl.SetMargins(walk.Margins{80, 20, 80, 20})
	vbl.SetSpacing(10)

	var disposables walk.Disposables
	defer disposables.Treat()

	var err error
	showingAboutDialog, err = walk.NewDialogWithFixedSize(owner)
	if err != nil {
		return err
	}
	defer func() {
		showingAboutDialog = nil
	}()
	disposables.Add(showingAboutDialog)
	showingAboutDialog.SetTitle("О Pinus Smart AWG")
	applyWindowChrome(showingAboutDialog.Handle())
	showingAboutDialog.SetLayout(vbl)
	background, _ := walk.NewSolidColorBrush(walk.RGB(7, 24, 43))
	if background != nil {
		defer background.Dispose()
		showingAboutDialog.SetBackground(background)
	}
	if icon, err := loadLogoIcon(32); err == nil {
		showingAboutDialog.SetIcon(icon)
	}

	font, _ := walk.NewFont("Verdana", 9, 0)
	if font != nil {
		defer font.Dispose()
	}
	showingAboutDialog.SetFont(font)

	iv, err := walk.NewImageView(showingAboutDialog)
	if err != nil {
		return err
	}
	iv.SetCursor(walk.CursorHand())
	iv.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			win.ShellExecute(showingAboutDialog.Handle(), nil, windows.StringToUTF16Ptr("https://t.me/MEN9_HET"), nil, nil, win.SW_SHOWNORMAL)
		} else if easterEggIndex >= 0 && button == walk.RightButton {
			if icon, err := loadSystemIcon("moricons", int32(easterEggIndex), 128); err == nil {
				iv.SetImage(icon)
				easterEggIndex++
			} else {
				easterEggIndex = -1
				if logo, err := loadLogoIcon(128); err == nil {
					iv.SetImage(logo)
				}
			}
		}
	})
	if logo, err := loadLogoIcon(128); err == nil {
		iv.SetImage(logo)
	}
	iv.Accessibility().SetName("Pinus Smart AWG logo image")

	wgLbl, err := walk.NewTextLabel(showingAboutDialog)
	if err != nil {
		return err
	}
	wgFont, _ := walk.NewFont("Verdana", 15, walk.FontBold)
	if wgFont != nil {
		defer wgFont.Dispose()
	}
	wgLbl.SetFont(wgFont)
	wgLbl.SetTextAlignment(walk.AlignHCenterVNear)
	wgLbl.SetText("Pinus Smart AWG")
	wgLbl.SetTextColor(walk.RGB(238, 247, 255))

	detailsLbl, err := walk.NewTextLabel(showingAboutDialog)
	if err != nil {
		return err
	}
	detailsLbl.SetTextAlignment(walk.AlignHCenterVNear)
	detailsLbl.SetText(l18n.Sprintf("Версия: %s\nGo: %s\nСистема: %s\nАрхитектура: %s", version.Number, strings.TrimPrefix(runtime.Version(), "go"), version.OsName(), version.Arch()))
	detailsLbl.SetTextColor(walk.RGB(176, 204, 231))

	copyrightLbl, err := walk.NewTextLabel(showingAboutDialog)
	if err != nil {
		return err
	}
	copyrightFont, _ := walk.NewFont("Verdana", 7, 0)
	if copyrightFont != nil {
		defer copyrightFont.Dispose()
	}
	copyrightLbl.SetFont(copyrightFont)
	copyrightLbl.SetTextAlignment(walk.AlignHCenterVNear)
	copyrightLbl.SetText("Pinus Smart AWG.\nОсновано на AmneziaWG, WireGuard и amnezia-box.")
	copyrightLbl.SetTextColor(walk.RGB(155, 188, 220))

	buttonCP, err := walk.NewComposite(showingAboutDialog)
	if err != nil {
		return err
	}
	hbl := walk.NewHBoxLayout()
	hbl.SetMargins(walk.Margins{VNear: 10})
	buttonCP.SetLayout(hbl)
	walk.NewHSpacer(buttonCP)
	closePB, err := walk.NewPushButton(buttonCP)
	if err != nil {
		return err
	}
	closePB.SetAlignment(walk.AlignHCenterVNear)
	closePB.SetText("Закрыть")
	closePB.Clicked().Attach(showingAboutDialog.Accept)
	_ = win.SetWindowTheme(closePB.Handle(), windows.StringToUTF16Ptr("DarkMode_Explorer"), nil)
	walk.NewHSpacer(buttonCP)

	showingAboutDialog.SetDefaultButton(closePB)
	showingAboutDialog.SetCancelButton(closePB)

	disposables.Spare()

	showingAboutDialog.Run()

	return nil
}
