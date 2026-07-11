/* SPDX-License-Identifier: MIT */

package ui

import (
	"path/filepath"
	"strings"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
)

type ruleRadioOption struct {
	box   *walk.Composite
	radio *walk.RadioButton
	label *walk.Label
}

func newRuleRadioOption(parent walk.Container, text string) ruleRadioOption {
	box, _ := walk.NewComposite(parent)
	layout := walk.NewHBoxLayout()
	layout.SetMargins(walk.Margins{})
	layout.SetSpacing(4)
	box.SetLayout(layout)
	radio, _ := walk.NewRadioButton(box)
	radio.SetText("")
	radio.SetMinMaxSize(walk.Size{18, 0}, walk.Size{18, 0})
	radio.SetCursor(walk.CursorHand())
	label, _ := walk.NewLabel(box)
	label.SetText(text)
	label.SetCursor(walk.CursorHand())
	layout.SetStretchFactor(label, 1)
	return ruleRadioOption{box: box, radio: radio, label: label}
}

func bindRuleRadioOptions(options []ruleRadioOption, changed func()) func(int) {
	updating := false
	refresh := func() {
		for _, option := range options {
			color := walk.RGB(155, 188, 220)
			if option.radio.Checked() {
				color = walk.RGB(238, 247, 255)
			}
			option.label.SetTextColor(color)
		}
	}
	selectIndex := func(index int) {
		if index < 0 || index >= len(options) {
			return
		}
		updating = true
		for i, option := range options {
			option.radio.SetChecked(i == index)
		}
		updating = false
		refresh()
		if changed != nil {
			changed()
		}
	}
	for i := range options {
		index := i
		options[i].radio.CheckedChanged().Attach(func() {
			if !updating && options[index].radio.Checked() {
				selectIndex(index)
			}
		})
		options[i].label.MouseUp().Attach(func(_, _ int, button walk.MouseButton) {
			if button == walk.LeftButton {
				selectIndex(index)
			}
		})
	}
	refresh()
	return selectIndex
}

func showLegacyRuleDialog(owner walk.Form, existing *smart.CustomRule) (smart.CustomRule, bool) {
	dialog, err := walk.NewDialog(owner)
	if err != nil {
		showErrorCustom(owner, "Не удалось открыть редактор", err.Error())
		return smart.CustomRule{}, false
	}
	defer dialog.Dispose()
	dialog.SetTitle("Правило маршрутизации")
	applyWindowChrome(dialog.Handle())
	if icon, iconErr := loadLogoIcon(32); iconErr == nil {
		dialog.SetIcon(icon)
	}
	dialog.SetSize(walk.Size{640, 450})
	dialog.SetMinMaxSize(walk.Size{580, 420}, walk.Size{760, 560})
	background, _ := walk.NewSolidColorBrush(walk.RGB(7, 24, 43))
	surface, _ := walk.NewSolidColorBrush(walk.RGB(18, 49, 79))
	if background != nil {
		defer background.Dispose()
		dialog.SetBackground(background)
	}
	if surface != nil {
		defer surface.Dispose()
	}
	font, _ := walk.NewFont("Verdana", 9, 0)
	if font != nil {
		defer font.Dispose()
		dialog.SetFont(font)
	}
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{18, 16, 18, 16})
	layout.SetSpacing(7)
	dialog.SetLayout(layout)

	title, _ := walk.NewTextLabel(dialog)
	title.SetText("Куда отправлять этот трафик")
	if heading, fontErr := walk.NewFont("Verdana", 12, walk.FontBold); fontErr == nil {
		defer heading.Dispose()
		title.SetFont(heading)
	}
	description, _ := walk.NewTextLabel(dialog)
	description.SetText("Правило применяется раньше выбранного общего режима.")

	nameLabel, _ := walk.NewLabel(dialog)
	nameLabel.SetText("Название")
	nameEdit, _ := walk.NewLineEdit(dialog)
	nameEdit.SetCueBanner("Например: Рабочий браузер")
	nameEdit.SetTextColor(walk.RGB(238, 247, 255))
	if surface != nil {
		nameEdit.SetBackground(surface)
	}

	typeLabel, _ := walk.NewLabel(dialog)
	typeLabel.SetText("Что маршрутизировать")
	typeRow, _ := walk.NewComposite(dialog)
	typeLayout := walk.NewHBoxLayout()
	typeLayout.SetMargins(walk.Margins{})
	typeLayout.SetSpacing(14)
	typeRow.SetLayout(typeLayout)
	appOption := newRuleRadioOption(typeRow, "Приложение")
	domainOption := newRuleRadioOption(typeRow, "Домен")
	cidrOption := newRuleRadioOption(typeRow, "IP / CIDR")
	typeOptions := []ruleRadioOption{appOption, domainOption, cidrOption}
	for _, option := range typeOptions {
		typeLayout.SetStretchFactor(option.box, 1)
	}

	valueLabel, _ := walk.NewLabel(dialog)
	valueLabel.SetText("Значение")
	valueRow, _ := walk.NewComposite(dialog)
	valueLayout := walk.NewHBoxLayout()
	valueLayout.SetMargins(walk.Margins{})
	valueLayout.SetSpacing(8)
	valueRow.SetLayout(valueLayout)
	valueEdit, _ := walk.NewLineEdit(valueRow)
	valueEdit.SetTextColor(walk.RGB(238, 247, 255))
	if surface != nil {
		valueEdit.SetBackground(surface)
	}
	valueLayout.SetStretchFactor(valueEdit, 1)
	runningButton, _ := walk.NewPushButton(valueRow)
	runningButton.SetText("Запущенные")
	runningButton.SetMinMaxSize(walk.Size{112, 0}, walk.Size{112, 0})
	browseButton, _ := walk.NewPushButton(valueRow)
	browseButton.SetText("Файл EXE")
	browseButton.SetMinMaxSize(walk.Size{94, 0}, walk.Size{94, 0})

	targetLabel, _ := walk.NewLabel(dialog)
	targetLabel.SetText("Направление")
	targetRow, _ := walk.NewComposite(dialog)
	targetLayout := walk.NewHBoxLayout()
	targetLayout.SetMargins(walk.Margins{})
	targetLayout.SetSpacing(14)
	targetRow.SetLayout(targetLayout)
	vpnOption := newRuleRadioOption(targetRow, "Через VPN")
	directOption := newRuleRadioOption(targetRow, "Напрямую")
	targetOptions := []ruleRadioOption{vpnOption, directOption}
	for _, option := range targetOptions {
		targetLayout.SetStretchFactor(option.box, 1)
	}

	enabled, _ := walk.NewCheckBox(dialog)
	enabled.SetText("Правило включено")
	enabled.SetChecked(true)

	walk.NewVSpacer(dialog)
	buttons, _ := walk.NewComposite(dialog)
	buttonLayout := walk.NewHBoxLayout()
	buttonLayout.SetMargins(walk.Margins{})
	buttonLayout.SetSpacing(8)
	buttons.SetLayout(buttonLayout)
	walk.NewHSpacer(buttons)
	cancelButton, _ := walk.NewPushButton(buttons)
	cancelButton.SetText("Отмена")
	cancelButton.SetMinMaxSize(walk.Size{100, 34}, walk.Size{100, 34})
	saveButton, _ := walk.NewPushButton(buttons)
	saveButton.SetText("Сохранить")
	saveButton.SetMinMaxSize(walk.Size{110, 34}, walk.Size{110, 34})
	dialog.SetCancelButton(cancelButton)
	dialog.SetDefaultButton(saveButton)

	updateValueCue := func() {
		switch {
		case domainOption.radio.Checked():
			valueEdit.SetCueBanner("example.com")
			runningButton.SetVisible(false)
			browseButton.SetVisible(false)
		case cidrOption.radio.Checked():
			valueEdit.SetCueBanner("1.1.1.1 или 203.0.113.0/24")
			runningButton.SetVisible(false)
			browseButton.SetVisible(false)
		default:
			valueEdit.SetCueBanner(`C:\Program Files\App\app.exe`)
			runningButton.SetVisible(true)
			browseButton.SetVisible(true)
		}
	}
	selectType := bindRuleRadioOptions(typeOptions, updateValueCue)
	selectTarget := bindRuleRadioOptions(targetOptions, nil)
	selectType(0)
	selectTarget(0)

	if existing != nil {
		nameEdit.SetText(existing.Name)
		valueEdit.SetText(existing.Value)
		enabled.SetChecked(existing.Enabled)
		switch existing.Kind {
		case smart.RuleDomain:
			selectType(1)
		case smart.RuleCIDR:
			selectType(2)
		default:
			selectType(0)
		}
		if existing.Target == smart.TargetDirect {
			selectTarget(1)
		}
		updateValueCue()
	}

	browseButton.Clicked().Attach(func() {
		fileDialog := walk.FileDialog{
			Filter: "Приложения Windows (*.exe)|*.exe|Все файлы (*.*)|*.*",
			Title:  "Выбери приложение",
		}
		if ok, _ := fileDialog.ShowOpen(dialog); ok {
			valueEdit.SetText(fileDialog.FilePath)
			if nameEdit.Text() == "" {
				nameEdit.SetText(strings.TrimSuffix(filepath.Base(fileDialog.FilePath), filepath.Ext(fileDialog.FilePath)))
			}
		}
	})
	runningButton.Clicked().Attach(func() {
		process, ok := showRunningProcessDialog(dialog)
		if !ok {
			return
		}
		valueEdit.SetText(process.routeValue())
		if nameEdit.Text() == "" {
			nameEdit.SetText(strings.TrimSuffix(process.Name, filepath.Ext(process.Name)))
		}
	})

	var result smart.CustomRule
	saveButton.Clicked().Attach(func() {
		result = smart.CustomRule{
			ID:      smart.NewRuleID(),
			Name:    nameEdit.Text(),
			Value:   valueEdit.Text(),
			Enabled: enabled.Checked(),
			Kind:    smart.RuleApplication,
			Target:  smart.TargetVPN,
		}
		if existing != nil && existing.ID != "" {
			result.ID = existing.ID
		}
		switch {
		case domainOption.radio.Checked():
			result.Kind = smart.RuleDomain
		case cidrOption.radio.Checked():
			result.Kind = smart.RuleCIDR
		}
		if directOption.radio.Checked() {
			result.Target = smart.TargetDirect
		}
		settings := smart.DefaultSettings()
		settings.CustomRules = []smart.CustomRule{result}
		normalized, normalizeErr := settings.Normalized()
		if normalizeErr != nil {
			walk.MsgBox(dialog, "Проверь правило", normalizeErr.Error(), walk.MsgBoxIconWarning)
			return
		}
		result = normalized.CustomRules[0]
		dialog.Accept()
	})
	cancelButton.Clicked().Attach(dialog.Cancel)

	for _, label := range []*walk.Label{nameLabel, typeLabel, valueLabel, targetLabel} {
		label.SetTextColor(walk.RGB(176, 204, 231))
	}
	title.SetTextColor(walk.RGB(238, 247, 255))
	description.SetTextColor(walk.RGB(155, 188, 220))
	darkTheme := windows.StringToUTF16Ptr("DarkMode_Explorer")
	for _, handle := range []win.HWND{
		dialog.Handle(), nameEdit.Handle(), valueEdit.Handle(), runningButton.Handle(), browseButton.Handle(), enabled.Handle(),
		appOption.radio.Handle(), domainOption.radio.Handle(), cidrOption.radio.Handle(),
		vpnOption.radio.Handle(), directOption.radio.Handle(),
		cancelButton.Handle(), saveButton.Handle(),
	} {
		_ = win.SetWindowTheme(handle, darkTheme, nil)
	}

	if dialog.Run() != walk.DlgCmdOK {
		return smart.CustomRule{}, false
	}
	return result, true
}
