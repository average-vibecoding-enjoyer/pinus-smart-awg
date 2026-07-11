/* SPDX-License-Identifier: MIT */

package ui

import (
	"path/filepath"
	"strings"

	"github.com/lxn/walk"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
)

func showRuleDialog(owner walk.Form, existing *smart.CustomRule, mode smart.Mode) (smart.CustomRule, bool) {
	dialog, err := walk.NewDialog(owner)
	if err != nil {
		showErrorCustom(owner, "Не удалось открыть редактор", err.Error())
		return smart.CustomRule{}, false
	}
	defer dialog.Dispose()
	dialog.SetTitle("Правило маршрутизации")
	dialog.SetSize(walk.Size{740, 620})
	dialog.SetMinMaxSize(walk.Size{700, 590}, walk.Size{860, 700})
	applyWindowChrome(dialog.Handle())
	if icon, iconErr := loadLogoIcon(32); iconErr == nil {
		dialog.SetIcon(icon)
	}

	kind := smart.RuleApplication
	target := smart.TargetVPN
	if smart.NormalizeMode(string(mode)) == smart.ModeAll {
		target = smart.TargetDirect
	}
	enabled := true
	if existing != nil {
		kind = existing.Kind
		target = existing.Target
		enabled = existing.Enabled
	}
	errorText := ""
	invalidValue := false
	var nameEdit, valueEdit *walk.LineEdit
	var surface *miniSurface
	var result smart.CustomRule

	isApplication := func() bool { return kind == smart.RuleApplication }
	valueCue := func() string {
		switch kind {
		case smart.RuleDomain:
			return "example.com"
		case smart.RuleCIDR:
			return "1.1.1.1 или 203.0.113.0/24"
		default:
			return `C:\Program Files\App\app.exe`
		}
	}
	valueFrameFor := func(bounds walk.Rectangle, ui *miniSurface) walk.Rectangle {
		margin := ui.px(24)
		width := bounds.Width - margin*2
		if isApplication() {
			width -= ui.px(236)
		}
		return walk.Rectangle{X: margin, Y: ui.px(264), Width: width, Height: ui.px(40)}
	}
	nameFrameFor := func(bounds walk.Rectangle, ui *miniSurface) walk.Rectangle {
		margin := ui.px(24)
		return walk.Rectangle{X: margin, Y: ui.px(104), Width: bounds.Width - margin*2, Height: ui.px(40)}
	}
	refreshEditorLayout := func() {
		if surface == nil || nameEdit == nil || valueEdit == nil {
			return
		}
		client := dialog.ClientBoundsPixels()
		_ = surface.SetBoundsPixels(client)
		placeMiniLineEdit(nameEdit, nameFrameFor(client, surface), surface)
		placeMiniLineEdit(valueEdit, valueFrameFor(client, surface), surface)
		valueEdit.SetCueBanner(valueCue())
		_ = surface.Invalidate()
	}

	save := func() {
		candidate := smart.CustomRule{
			ID:      smart.NewRuleID(),
			Name:    nameEdit.Text(),
			Kind:    kind,
			Value:   valueEdit.Text(),
			Target:  target,
			Enabled: enabled,
		}
		if existing != nil && existing.ID != "" {
			candidate.ID = existing.ID
		}
		settings := smart.DefaultSettings()
		settings.Mode = mode
		settings.CustomRules = []smart.CustomRule{candidate}
		normalized, normalizeErr := settings.Normalized()
		if normalizeErr != nil {
			errorText = normalizeErr.Error()
			invalidValue = true
			_ = valueEdit.SetFocus()
			_ = surface.Invalidate()
			return
		}
		result = normalized.CustomRules[0]
		dialog.Accept()
	}

	paint := func(ui *miniSurface, canvas *walk.Canvas, bounds walk.Rectangle) {
		margin := ui.px(24)
		title := "Новое правило маршрутизации"
		if existing != nil {
			title = "Редактирование правила"
		}
		ui.drawText(canvas, title, ui.theme.titleFont, ui.theme.textColor, walk.Rectangle{X: margin, Y: ui.px(17), Width: bounds.Width - margin*2, Height: ui.px(34)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		subtitle := "Правило имеет приоритет над выбранным общим режимом."
		if smart.NormalizeMode(string(mode)) == smart.ModeAll {
			subtitle = "Весь интернет останется под VPN, а направление «Напрямую» создаст исключение."
		}
		ui.drawText(canvas, subtitle, ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(51), Width: bounds.Width - margin*2, Height: ui.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

		ui.drawText(canvas, "Название", ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(78), Width: bounds.Width - margin*2, Height: ui.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		nameFrame := nameFrameFor(bounds, ui)
		ui.drawFieldFrame(canvas, "rule:focus:name", nameFrame, nameEdit != nil && nameEdit.Focused(), false)

		ui.drawText(canvas, "Что маршрутизировать", ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(154), Width: bounds.Width - margin*2, Height: ui.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		gap := ui.px(10)
		segmentWidth := (bounds.Width - margin*2 - gap*2) / 3
		typeY := ui.px(180)
		ui.drawSegment(canvas, "rule:type:app", "Приложение", "EXE или процесс", walk.Rectangle{X: margin, Y: typeY, Width: segmentWidth, Height: ui.px(48)}, kind == smart.RuleApplication, false)
		ui.drawSegment(canvas, "rule:type:domain", "Домен", "Сайт и поддомены", walk.Rectangle{X: margin + segmentWidth + gap, Y: typeY, Width: segmentWidth, Height: ui.px(48)}, kind == smart.RuleDomain, false)
		ui.drawSegment(canvas, "rule:type:cidr", "IP / CIDR", "Адрес или сеть", walk.Rectangle{X: margin + (segmentWidth+gap)*2, Y: typeY, Width: segmentWidth, Height: ui.px(48)}, kind == smart.RuleCIDR, false)

		ui.drawText(canvas, "Значение", ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(238), Width: bounds.Width - margin*2, Height: ui.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		valueFrame := valueFrameFor(bounds, ui)
		ui.drawFieldFrame(canvas, "rule:focus:value", valueFrame, valueEdit != nil && valueEdit.Focused(), invalidValue)
		if isApplication() {
			buttonX := valueFrame.X + valueFrame.Width + ui.px(10)
			ui.drawButton(canvas, "rule:running", "Запущенные", "\ue7c4", walk.Rectangle{X: buttonX, Y: valueFrame.Y, Width: ui.px(120), Height: valueFrame.Height}, false, false)
			ui.drawButton(canvas, "rule:file", "Файл EXE", "\ue8b7", walk.Rectangle{X: buttonX + ui.px(130), Y: valueFrame.Y, Width: ui.px(106), Height: valueFrame.Height}, false, false)
		}

		ui.drawText(canvas, "Направление", ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(318), Width: bounds.Width - margin*2, Height: ui.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		directionY := ui.px(342)
		directionWidth := (bounds.Width - margin*2 - gap) / 2
		ui.drawSegment(canvas, "rule:target:vpn", "Через VPN", "Принудительно через туннель", walk.Rectangle{X: margin, Y: directionY, Width: directionWidth, Height: ui.px(56)}, target == smart.TargetVPN, false)
		ui.drawSegment(canvas, "rule:target:direct", "Напрямую", "В обход VPN", walk.Rectangle{X: margin + directionWidth + gap, Y: directionY, Width: directionWidth, Height: ui.px(56)}, target == smart.TargetDirect, false)

		enabledRow := walk.Rectangle{X: margin, Y: ui.px(412), Width: bounds.Width - margin*2, Height: ui.px(34)}
		ui.drawText(canvas, "Правило включено", ui.theme.bodyFont, ui.theme.textColor, walk.Rectangle{X: enabledRow.X, Y: enabledRow.Y, Width: enabledRow.Width - ui.px(72), Height: enabledRow.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		toggleBounds := walk.Rectangle{X: enabledRow.X + enabledRow.Width - ui.px(48), Y: enabledRow.Y + ui.px(5), Width: ui.px(48), Height: ui.px(24)}
		ui.drawToggle(canvas, "rule:enabled", toggleBounds, enabled, false)
		ui.addHit("rule:enabled", enabledRow, false)

		if errorText != "" {
			ui.drawText(canvas, errorText, ui.theme.smallFont, ui.theme.dangerColor, walk.Rectangle{X: margin, Y: ui.px(450), Width: bounds.Width - margin*2, Height: ui.px(36)}, walk.TextLeft|walk.TextVCenter|walk.TextWordbreak|walk.TextEndEllipsis)
		}

		footerY := bounds.Height - ui.px(62)
		buttonWidth := ui.px(112)
		ui.drawButton(canvas, "rule:cancel", "Отмена", "", walk.Rectangle{X: bounds.Width - margin - buttonWidth*2 - ui.px(10), Y: footerY, Width: buttonWidth, Height: ui.px(40)}, false, false)
		ui.drawButton(canvas, "rule:save", "Сохранить", "\ue74e", walk.Rectangle{X: bounds.Width - margin - buttonWidth, Y: footerY, Width: buttonWidth, Height: ui.px(40)}, true, false)
	}

	activate := func(id string) {
		switch id {
		case "rule:focus:name":
			_ = nameEdit.SetFocus()
		case "rule:focus:value":
			_ = valueEdit.SetFocus()
		case "rule:type:app":
			kind = smart.RuleApplication
			invalidValue = false
			errorText = ""
			refreshEditorLayout()
		case "rule:type:domain":
			kind = smart.RuleDomain
			invalidValue = false
			errorText = ""
			refreshEditorLayout()
		case "rule:type:cidr":
			kind = smart.RuleCIDR
			invalidValue = false
			errorText = ""
			refreshEditorLayout()
		case "rule:target:vpn":
			target = smart.TargetVPN
			_ = surface.Invalidate()
		case "rule:target:direct":
			target = smart.TargetDirect
			_ = surface.Invalidate()
		case "rule:enabled":
			enabled = !enabled
			_ = surface.Invalidate()
		case "rule:running":
			process, ok := showRunningProcessDialog(dialog)
			if !ok {
				return
			}
			valueEdit.SetText(process.routeValue())
			if strings.TrimSpace(nameEdit.Text()) == "" {
				nameEdit.SetText(strings.TrimSuffix(process.Name, filepath.Ext(process.Name)))
			}
		case "rule:file":
			fileDialog := walk.FileDialog{Filter: "Приложения Windows (*.exe)|*.exe|Все файлы (*.*)|*.*", Title: "Выбери приложение"}
			if ok, _ := fileDialog.ShowOpen(dialog); ok {
				valueEdit.SetText(fileDialog.FilePath)
				if strings.TrimSpace(nameEdit.Text()) == "" {
					nameEdit.SetText(strings.TrimSuffix(filepath.Base(fileDialog.FilePath), filepath.Ext(fileDialog.FilePath)))
				}
			}
		case "rule:cancel":
			dialog.Cancel()
		case "rule:save":
			save()
		}
	}

	surface, err = newMiniSurface(dialog, paint, activate)
	if err != nil {
		return smart.CustomRule{}, false
	}
	dialog.SetBackground(surface.theme.background)
	nameEdit, err = walk.NewLineEdit(dialog)
	if err != nil {
		return smart.CustomRule{}, false
	}
	valueEdit, err = walk.NewLineEdit(dialog)
	if err != nil {
		return smart.CustomRule{}, false
	}
	nameEdit.SetCueBanner("Например: Рабочий браузер")
	valueEdit.SetCueBanner(valueCue())
	styleMiniLineEdit(nameEdit, surface)
	styleMiniLineEdit(valueEdit, surface)
	if err = detachMiniOverlayWidget(dialog, nameEdit); err != nil {
		return smart.CustomRule{}, false
	}
	defer nameEdit.Dispose()
	if err = detachMiniOverlayWidget(dialog, valueEdit); err != nil {
		return smart.CustomRule{}, false
	}
	defer valueEdit.Dispose()
	if err = installMiniDialogLayout(dialog); err != nil {
		return smart.CustomRule{}, false
	}
	if existing != nil {
		nameEdit.SetText(existing.Name)
		valueEdit.SetText(existing.Value)
	}
	clearError := func() {
		if errorText != "" || invalidValue {
			errorText = ""
			invalidValue = false
			_ = surface.Invalidate()
		}
	}
	nameEdit.TextChanged().Attach(clearError)
	valueEdit.TextChanged().Attach(clearError)
	dialog.SizeChanged().Attach(refreshEditorLayout)
	refreshEditorLayout()

	handleKey := func(key walk.Key) {
		switch key {
		case walk.KeyEscape:
			dialog.Cancel()
		case walk.KeyReturn:
			save()
		}
	}
	surface.KeyDown().Attach(handleKey)
	nameEdit.KeyDown().Attach(handleKey)
	valueEdit.KeyDown().Attach(handleKey)

	if dialog.Run() != walk.DlgCmdOK {
		return smart.CustomRule{}, false
	}
	return result, true
}
