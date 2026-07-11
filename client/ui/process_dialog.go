/* SPDX-License-Identifier: MIT */

package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

type runningProcess struct {
	Name      string
	Path      string
	Instances int
}

func (process runningProcess) routeValue() string {
	if process.Path != "" {
		return process.Path
	}
	return process.Name
}

func mergeRunningProcesses(values []runningProcess) []runningProcess {
	merged := make([]runningProcess, 0, len(values))
	indexes := make(map[string]int, len(values))
	for _, process := range values {
		process.Name = strings.TrimSpace(process.Name)
		process.Path = strings.Trim(strings.TrimSpace(process.Path), "\"")
		if process.Name == "" && process.Path != "" {
			process.Name = filepath.Base(process.Path)
		}
		if process.Name == "" {
			continue
		}
		if process.Instances < 1 {
			process.Instances = 1
		}
		key := strings.ToLower(process.Path)
		if key == "" {
			key = "name:" + strings.ToLower(process.Name)
		}
		if index, ok := indexes[key]; ok {
			merged[index].Instances += process.Instances
			continue
		}
		indexes[key] = len(merged)
		merged = append(merged, process)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		left := strings.ToLower(merged[i].Name)
		right := strings.ToLower(merged[j].Name)
		if left == right {
			return strings.ToLower(merged[i].Path) < strings.ToLower(merged[j].Path)
		}
		return left < right
	})
	return merged
}

func filterRunningProcesses(values []runningProcess, query string) []runningProcess {
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(terms) == 0 {
		return append([]runningProcess(nil), values...)
	}
	filtered := make([]runningProcess, 0, len(values))
	for _, process := range values {
		haystack := strings.ToLower(process.Name + " " + process.Path)
		matches := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, process)
		}
	}
	return filtered
}

func executablePathForProcess(processID uint32) string {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}

func enumerateRunningProcesses() ([]runningProcess, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить список процессов: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать список процессов: %w", err)
	}

	processes := make([]runningProcess, 0, 128)
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if name != "" && entry.ProcessID > 4 {
			processes = append(processes, runningProcess{
				Name:      name,
				Path:      executablePathForProcess(entry.ProcessID),
				Instances: 1,
			})
		}
		err = windows.Process32Next(snapshot, &entry)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("не удалось дочитать список процессов: %w", err)
		}
	}
	return mergeRunningProcesses(processes), nil
}

type runningProcessModel struct {
	walk.ListModelBase
	all      []runningProcess
	filtered []runningProcess
}

func (model *runningProcessModel) ItemCount() int {
	return len(model.filtered)
}

func (model *runningProcessModel) Value(index int) any {
	if index < 0 || index >= len(model.filtered) {
		return ""
	}
	process := model.filtered[index]
	path := process.Path
	if path == "" {
		path = "путь недоступен, будет использовано имя процесса"
	}
	return fmt.Sprintf("%s  |  %d  |  %s", process.Name, process.Instances, path)
}

func (model *runningProcessModel) replace(values []runningProcess, query string) {
	model.all = append([]runningProcess(nil), values...)
	model.filter(query)
}

func (model *runningProcessModel) filter(query string) {
	model.filtered = filterRunningProcesses(model.all, query)
	model.PublishItemsReset()
}

func showLegacyRunningProcessDialog(owner walk.Form) (runningProcess, bool) {
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
	applyWindowChrome(dialog.Handle())
	if icon, iconErr := loadLogoIcon(32); iconErr == nil {
		dialog.SetIcon(icon)
	}
	dialog.SetSize(walk.Size{780, 540})
	dialog.SetMinMaxSize(walk.Size{640, 430}, walk.Size{980, 720})

	background, _ := walk.NewSolidColorBrush(walk.RGB(7, 24, 43))
	surface, _ := walk.NewSolidColorBrush(walk.RGB(18, 49, 79))
	listBackground, _ := walk.NewSolidColorBrush(walk.RGB(245, 248, 252))
	if background != nil {
		defer background.Dispose()
		dialog.SetBackground(background)
	}
	if surface != nil {
		defer surface.Dispose()
	}
	if listBackground != nil {
		defer listBackground.Dispose()
	}
	font, _ := walk.NewFont("Verdana", 9, 0)
	if font != nil {
		defer font.Dispose()
		dialog.SetFont(font)
	}
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{18, 16, 18, 16})
	layout.SetSpacing(8)
	dialog.SetLayout(layout)

	title, _ := walk.NewTextLabel(dialog)
	title.SetText("Выбери работающий процесс")
	title.SetTextColor(walk.RGB(238, 247, 255))
	description, _ := walk.NewTextLabel(dialog)
	description.SetText("Одинаковые экземпляры объединены. Поиск работает по имени и полному пути.")
	description.SetTextColor(walk.RGB(155, 188, 220))

	searchRow, _ := walk.NewComposite(dialog)
	searchLayout := walk.NewHBoxLayout()
	searchLayout.SetMargins(walk.Margins{})
	searchLayout.SetSpacing(8)
	searchRow.SetLayout(searchLayout)
	searchEdit, _ := walk.NewLineEdit(searchRow)
	searchEdit.SetCueBanner("Поиск процесса")
	searchEdit.SetTextColor(walk.RGB(238, 247, 255))
	if surface != nil {
		searchEdit.SetBackground(surface)
	}
	searchLayout.SetStretchFactor(searchEdit, 1)
	refreshButton, _ := walk.NewPushButton(searchRow)
	refreshButton.SetText("Обновить")
	refreshButton.SetMinMaxSize(walk.Size{104, 32}, walk.Size{104, 32})

	listHint, _ := walk.NewTextLabel(dialog)
	listHint.SetText("Приложение  |  экземпляров  |  полный путь")
	listHint.SetTextColor(walk.RGB(155, 188, 220))
	processList, _ := walk.NewListBox(dialog)
	if listBackground != nil {
		processList.SetBackground(listBackground)
	}
	_ = win.SetWindowTheme(processList.Handle(), windows.StringToUTF16Ptr("Explorer"), nil)
	model := &runningProcessModel{}
	model.replace(processes, "")
	_ = processList.SetModel(model)

	countLabel, _ := walk.NewLabel(dialog)
	countLabel.SetTextColor(walk.RGB(155, 188, 220))

	buttons, _ := walk.NewComposite(dialog)
	buttonLayout := walk.NewHBoxLayout()
	buttonLayout.SetMargins(walk.Margins{})
	buttonLayout.SetSpacing(8)
	buttons.SetLayout(buttonLayout)
	walk.NewHSpacer(buttons)
	cancelButton, _ := walk.NewPushButton(buttons)
	cancelButton.SetText("Отмена")
	cancelButton.SetMinMaxSize(walk.Size{100, 34}, walk.Size{100, 34})
	chooseButton, _ := walk.NewPushButton(buttons)
	chooseButton.SetText("Выбрать")
	chooseButton.SetMinMaxSize(walk.Size{110, 34}, walk.Size{110, 34})
	dialog.SetCancelButton(cancelButton)
	dialog.SetDefaultButton(chooseButton)

	updateSelection := func() {
		index := processList.CurrentIndex()
		chooseButton.SetEnabled(index >= 0 && index < len(model.filtered))
		countLabel.SetText(fmt.Sprintf("Найдено: %d", len(model.filtered)))
	}
	applyFilter := func() {
		model.filter(searchEdit.Text())
		if len(model.filtered) > 0 {
			_ = processList.SetCurrentIndex(0)
		}
		updateSelection()
	}
	searchEdit.TextChanged().Attach(applyFilter)
	refreshButton.Clicked().Attach(func() {
		values, refreshErr := enumerateRunningProcesses()
		if refreshErr != nil {
			walk.MsgBox(dialog, "Запущенные приложения", refreshErr.Error(), walk.MsgBoxIconError)
			return
		}
		model.replace(values, searchEdit.Text())
		if len(model.filtered) > 0 {
			_ = processList.SetCurrentIndex(0)
		}
		updateSelection()
	})
	processList.CurrentIndexChanged().Attach(updateSelection)

	var selected runningProcess
	accept := func() {
		index := processList.CurrentIndex()
		if index < 0 || index >= len(model.filtered) {
			return
		}
		selected = model.filtered[index]
		dialog.Accept()
	}
	chooseButton.Clicked().Attach(accept)
	processList.ItemActivated().Attach(accept)
	cancelButton.Clicked().Attach(dialog.Cancel)

	darkTheme := windows.StringToUTF16Ptr("DarkMode_Explorer")
	for _, handle := range []win.HWND{
		dialog.Handle(), searchEdit.Handle(), refreshButton.Handle(), cancelButton.Handle(), chooseButton.Handle(),
	} {
		_ = win.SetWindowTheme(handle, darkTheme, nil)
	}
	if len(model.filtered) > 0 {
		_ = processList.SetCurrentIndex(0)
	}
	updateSelection()

	if dialog.Run() != walk.DlgCmdOK {
		return runningProcess{}, false
	}
	return selected, true
}
