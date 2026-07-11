/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2022 WireGuard LLC. All Rights Reserved.
 */

package ui

import (
	"runtime"
	"strings"

	"github.com/lxn/walk"

	"github.com/amnezia-vpn/amneziawg-windows-client/version"
)

var showingAboutDialog *walk.Dialog

func onAbout(owner walk.Form) {
	showError(runAboutDialog(owner), owner)
}

func runAboutDialog(owner walk.Form) error {
	if showingAboutDialog != nil {
		showingAboutDialog.Show()
		raise(showingAboutDialog.Handle())
		return nil
	}

	dialog, err := walk.NewDialog(owner)
	if err != nil {
		return err
	}
	showingAboutDialog = dialog
	defer func() { showingAboutDialog = nil }()
	defer dialog.Dispose()

	dialog.SetTitle("О Pinus Smart AWG")
	dialog.SetSize(walk.Size{Width: 500, Height: 540})
	dialog.SetMinMaxSize(walk.Size{Width: 500, Height: 540}, walk.Size{Width: 500, Height: 540})
	applyWindowChrome(dialog.Handle())
	if icon, iconErr := loadLogoIcon(32); iconErr == nil {
		dialog.SetIcon(icon)
	}
	logo, _ := loadLogoIcon(128)

	rows := []struct {
		label string
		value string
	}{
		{"Версия", version.Number},
		{"Go", strings.TrimPrefix(runtime.Version(), "go")},
		{"Система", version.OsName()},
		{"Архитектура", version.Arch()},
	}

	paint := func(ui *miniSurface, canvas *walk.Canvas, bounds walk.Rectangle) {
		margin := ui.px(24)
		logoSize := ui.px(96)
		logoBounds := walk.Rectangle{X: (bounds.Width - logoSize) / 2, Y: ui.px(18), Width: logoSize, Height: logoSize}
		if logo != nil {
			ui.drawImage(canvas, logo, logoBounds)
		}

		ui.drawText(canvas, "Pinus Smart AWG", ui.theme.titleFont, ui.theme.textColor, walk.Rectangle{X: margin, Y: ui.px(124), Width: bounds.Width - margin*2, Height: ui.px(34)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		ui.drawText(canvas, "VPN и умная маршрутизация в одном окне", ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(158), Width: bounds.Width - margin*2, Height: ui.px(22)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

		infoCard := walk.Rectangle{X: margin, Y: ui.px(196), Width: bounds.Width - margin*2, Height: ui.px(120)}
		ui.drawCard(canvas, "", infoCard, false, false)
		rowHeight := ui.px(26)
		rowY := infoCard.Y + ui.px(8)
		for index, row := range rows {
			if index > 0 {
				line := walk.Rectangle{X: infoCard.X + ui.px(16), Y: rowY, Width: infoCard.Width - ui.px(32), Height: ui.px(1)}
				ui.fillGradient(canvas, line, ui.theme.border.Color(), ui.theme.border.Color(), 0, ui.theme.border)
			}
			ui.drawText(canvas, row.label, ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: infoCard.X + ui.px(18), Y: rowY, Width: ui.px(116), Height: rowHeight}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
			ui.drawText(canvas, row.value, ui.theme.bodyFont, ui.theme.textColor, walk.Rectangle{X: infoCard.X + ui.px(142), Y: rowY, Width: infoCard.Width - ui.px(160), Height: rowHeight}, walk.TextRight|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			rowY += rowHeight
		}

		ui.drawText(canvas, "Разработано командой Pinus VPN", ui.theme.headingFont, ui.theme.textColor, walk.Rectangle{X: margin, Y: ui.px(330), Width: bounds.Width - margin*2, Height: ui.px(26)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		ui.drawButton(canvas, "about:telegram", "Открыть @pinusvpn_bot", "\ue8f2", walk.Rectangle{X: margin, Y: ui.px(362), Width: bounds.Width - margin*2, Height: ui.px(46)}, true, false)

		ui.drawText(canvas, "Основано на AmneziaWG, WireGuard и amnezia-box.", ui.theme.microFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(418), Width: bounds.Width - margin*2, Height: ui.px(26)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

		closeWidth := ui.px(112)
		ui.drawButton(canvas, "about:close", "Закрыть", "", walk.Rectangle{X: bounds.Width - margin - closeWidth, Y: bounds.Height - ui.px(56), Width: closeWidth, Height: ui.px(40)}, false, false)
	}

	activate := func(id string) {
		switch id {
		case "about:telegram":
			openPinusVPNBot(dialog)
		case "about:close":
			dialog.Accept()
		}
	}

	surface, err := newMiniSurface(dialog, paint, activate)
	if err != nil {
		return err
	}
	if err = installMiniDialogLayout(dialog); err != nil {
		return err
	}
	dialog.SetBackground(surface.theme.background)
	surface.KeyDown().Attach(func(key walk.Key) {
		if key == walk.KeyEscape || key == walk.KeyReturn {
			dialog.Accept()
		}
	})

	dialog.Run()
	return nil
}
