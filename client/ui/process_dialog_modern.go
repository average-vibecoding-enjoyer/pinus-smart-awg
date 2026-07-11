/* SPDX-License-Identifier: MIT */

package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lxn/walk"
)

func showRunningProcessDialog(owner walk.Form) (runningProcess, bool) {
	processes, err := enumerateRunningProcesses()
	if err != nil {
		walk.MsgBox(owner, "Запущенные приложения", err.Error(), walk.MsgBoxIconError)
		return runningProcess{}, false
	}

	dialog, err := walk.NewDialog(owner)
	if err != nil {
		return runningProcess{}, false
	}
	defer dialog.Dispose()
	dialog.SetTitle("Запущенные приложения")
	dialog.SetSize(walk.Size{780, 600})
	dialog.SetMinMaxSize(walk.Size{700, 540}, walk.Size{940, 720})
	applyWindowChrome(dialog.Handle())
	if icon, iconErr := loadLogoIcon(32); iconErr == nil {
		dialog.SetIcon(icon)
	}

	all := append([]runningProcess(nil), processes...)
	filtered := append([]runningProcess(nil), processes...)
	selected := -1
	if len(filtered) > 0 {
		selected = 0
	}
	scroll := 0
	statusText := ""
	var searchEdit *walk.LineEdit
	var surface *miniSurface

	visibleRows := func() int {
		if surface == nil {
			return 1
		}
		height := surface.ClientBoundsPixels().Height - surface.px(224)
		rows := height / surface.px(52)
		if rows < 1 {
			rows = 1
		}
		return rows
	}
	clampScroll := func() {
		maximum := len(filtered) - visibleRows()
		if maximum < 0 {
			maximum = 0
		}
		if scroll < 0 {
			scroll = 0
		}
		if scroll > maximum {
			scroll = maximum
		}
	}
	applySearch := func() {
		query := ""
		if searchEdit != nil {
			query = searchEdit.Text()
		}
		filtered = filterRunningProcesses(all, query)
		scroll = 0
		selected = -1
		if len(filtered) > 0 {
			selected = 0
		}
		statusText = ""
		if surface != nil {
			_ = surface.Invalidate()
		}
	}
	accept := func() {
		if selected < 0 || selected >= len(filtered) {
			return
		}
		dialog.Accept()
	}

	paint := func(ui *miniSurface, canvas *walk.Canvas, bounds walk.Rectangle) {
		margin := ui.px(22)
		ui.drawText(canvas, "Выбери запущенное приложение", ui.theme.titleFont, ui.theme.textColor, walk.Rectangle{X: margin, Y: ui.px(18), Width: bounds.Width - margin*2, Height: ui.px(32)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		ui.drawText(canvas, "Поиск работает по имени и полному пути. Одинаковые процессы объединены.", ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: ui.px(51), Width: bounds.Width - margin*2, Height: ui.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

		refreshWidth := ui.px(116)
		searchFrame := walk.Rectangle{X: margin, Y: ui.px(88), Width: bounds.Width - margin*2 - refreshWidth - ui.px(10), Height: ui.px(40)}
		focused := searchEdit != nil && searchEdit.Focused()
		ui.drawFieldFrame(canvas, "process:search", searchFrame, focused, false)
		ui.drawButton(canvas, "process:refresh", "Обновить", "\ue72c", walk.Rectangle{X: searchFrame.X + searchFrame.Width + ui.px(10), Y: searchFrame.Y, Width: refreshWidth, Height: searchFrame.Height}, false, false)

		listTop := ui.px(146)
		footerTop := bounds.Height - ui.px(66)
		listBounds := walk.Rectangle{X: margin, Y: listTop, Width: bounds.Width - margin*2, Height: footerTop - listTop - ui.px(14)}
		ui.borderedCard(canvas, listBounds, ui.theme.surface, ui.theme.border.Color(), ui.px(8), ui.px(1))

		rowHeight := ui.px(52)
		rows := visibleRows()
		clampScroll()
		if len(filtered) == 0 {
			ui.drawText(canvas, "Ничего не найдено", ui.theme.headingFont, ui.theme.textColor, walk.Rectangle{X: listBounds.X + ui.px(20), Y: listBounds.Y + listBounds.Height/2 - ui.px(24), Width: listBounds.Width - ui.px(40), Height: ui.px(24)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
			ui.drawText(canvas, "Измени запрос или обнови список процессов", ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: listBounds.X + ui.px(20), Y: listBounds.Y + listBounds.Height/2, Width: listBounds.Width - ui.px(40), Height: ui.px(22)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		} else {
			end := scroll + rows
			if end > len(filtered) {
				end = len(filtered)
			}
			for index := scroll; index < end; index++ {
				process := filtered[index]
				y := listBounds.Y + ui.px(5) + (index-scroll)*rowHeight
				row := walk.Rectangle{X: listBounds.X + ui.px(5), Y: y, Width: listBounds.Width - ui.px(10), Height: rowHeight - ui.px(4)}
				id := "process:item:" + strconv.Itoa(index)
				ui.drawCard(canvas, id, row, index == selected, false)
				ui.drawText(canvas, "\ue8b7", ui.theme.iconFont, ui.theme.accentColor, walk.Rectangle{X: row.X + ui.px(12), Y: row.Y, Width: ui.px(24), Height: row.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
				ui.drawText(canvas, process.Name, ui.theme.headingFont, ui.theme.textColor, walk.Rectangle{X: row.X + ui.px(44), Y: row.Y + ui.px(5), Width: row.Width - ui.px(180), Height: ui.px(21)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
				path := process.Path
				if path == "" {
					path = "Путь недоступен, будет использовано имя процесса"
				}
				ui.drawText(canvas, path, ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: row.X + ui.px(44), Y: row.Y + ui.px(25), Width: row.Width - ui.px(158), Height: ui.px(20)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextPathEllipsis)
				pillWidth := ui.px(76)
				ui.drawPill(canvas, fmt.Sprintf("%d запущ.", process.Instances), walk.Rectangle{X: row.X + row.Width - pillWidth - ui.px(12), Y: row.Y + ui.px(12), Width: pillWidth, Height: ui.px(24)}, index == selected)
			}
		}

		if len(filtered) > rows {
			track := walk.Rectangle{X: listBounds.X + listBounds.Width - ui.px(7), Y: listBounds.Y + ui.px(8), Width: ui.px(3), Height: listBounds.Height - ui.px(16)}
			ui.fillRounded(canvas, ui.theme.faint.Color(), ui.theme.faint, track, ui.px(2))
			thumbHeight := track.Height * rows / len(filtered)
			if thumbHeight < ui.px(28) {
				thumbHeight = ui.px(28)
			}
			maximum := len(filtered) - rows
			thumbY := track.Y
			if maximum > 0 {
				thumbY += (track.Height - thumbHeight) * scroll / maximum
			}
			thumb := walk.Rectangle{X: track.X, Y: thumbY, Width: track.Width, Height: thumbHeight}
			ui.fillRounded(canvas, ui.theme.accentSoft.Color(), ui.theme.accentSoft, thumb, ui.px(2))
		}

		countText := fmt.Sprintf("Найдено: %d", len(filtered))
		if statusText != "" {
			countText = statusText
		}
		ui.drawText(canvas, countText, ui.theme.smallFont, ui.theme.mutedColor, walk.Rectangle{X: margin, Y: footerTop, Width: bounds.Width - margin*2 - ui.px(250), Height: ui.px(40)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		buttonWidth := ui.px(108)
		ui.drawButton(canvas, "process:cancel", "Отмена", "", walk.Rectangle{X: bounds.Width - margin - buttonWidth*2 - ui.px(10), Y: footerTop, Width: buttonWidth, Height: ui.px(40)}, false, false)
		ui.drawButton(canvas, "process:select", "Выбрать", "\ue73e", walk.Rectangle{X: bounds.Width - margin - buttonWidth, Y: footerTop, Width: buttonWidth, Height: ui.px(40)}, true, selected < 0 || selected >= len(filtered))
	}

	activate := func(id string) {
		switch id {
		case "process:search":
			if searchEdit != nil {
				_ = searchEdit.SetFocus()
			}
		case "process:refresh":
			values, refreshErr := enumerateRunningProcesses()
			if refreshErr != nil {
				statusText = refreshErr.Error()
				_ = surface.Invalidate()
				return
			}
			all = values
			applySearch()
		case "process:cancel":
			dialog.Cancel()
		case "process:select":
			accept()
		default:
			if strings.HasPrefix(id, "process:item:") {
				index, parseErr := strconv.Atoi(strings.TrimPrefix(id, "process:item:"))
				if parseErr == nil && index >= 0 && index < len(filtered) {
					selected = index
					_ = surface.Invalidate()
				}
			}
		}
	}

	surface, err = newMiniSurface(dialog, paint, activate)
	if err != nil {
		return runningProcess{}, false
	}
	dialog.SetBackground(surface.theme.background)
	searchEdit, err = walk.NewLineEdit(dialog)
	if err != nil {
		return runningProcess{}, false
	}
	searchEdit.SetCueBanner("Поиск по имени или пути")
	styleMiniLineEdit(searchEdit, surface)
	if err = detachMiniOverlayWidget(dialog, searchEdit); err != nil {
		return runningProcess{}, false
	}
	defer searchEdit.Dispose()
	if err = installMiniDialogLayout(dialog); err != nil {
		return runningProcess{}, false
	}
	searchEdit.TextChanged().Attach(applySearch)

	layoutChildren := func() {
		client := dialog.ClientBoundsPixels()
		_ = surface.SetBoundsPixels(client)
		margin := surface.px(22)
		refreshWidth := surface.px(116)
		frame := walk.Rectangle{X: margin, Y: surface.px(88), Width: client.Width - margin*2 - refreshWidth - surface.px(10), Height: surface.px(40)}
		placeMiniLineEdit(searchEdit, frame, surface)
	}
	dialog.SizeChanged().Attach(layoutChildren)
	layoutChildren()

	handleKey := func(key walk.Key) {
		switch key {
		case walk.KeyEscape:
			dialog.Cancel()
		case walk.KeyReturn:
			accept()
		}
	}
	surface.KeyDown().Attach(handleKey)
	searchEdit.KeyDown().Attach(handleKey)
	surface.wheel = func(delta int) {
		if delta > 0 {
			scroll--
		} else if delta < 0 {
			scroll++
		}
		clampScroll()
		_ = surface.Invalidate()
	}

	if dialog.Run() != walk.DlgCmdOK || selected < 0 || selected >= len(filtered) {
		return runningProcess{}, false
	}
	return filtered[selected], true
}
