package ui

import (
	"errors"
	"fmt"
	"github.com/amnezia-vpn/amneziawg-windows-client/manager"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
	"github.com/lxn/walk"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func toolDialog(owner walk.Form, title string, height int) (*walk.Dialog, error) {
	d, err := walk.NewDialog(owner)
	if err != nil {
		return nil, err
	}
	d.SetTitle(title)
	d.SetSize(walk.Size{650, height})
	d.SetLayout(walk.NewVBoxLayout())
	return d, nil
}
func toolLabel(parent walk.Container, text string) {
	l, err := walk.NewLabel(parent)
	if err == nil {
		l.SetText(text)
	}
}
func toolButton(parent walk.Container, text string, action func()) *walk.PushButton {
	b, err := walk.NewPushButton(parent)
	if err != nil {
		return nil
	}
	b.SetText(text)
	b.Clicked().Attach(action)
	return b
}
func showToolText(owner walk.Form, title, text string) {
	d, err := toolDialog(owner, title, 500)
	if err != nil {
		return
	}
	defer d.Dispose()
	view, err := walk.NewTextEdit(d)
	if err != nil {
		return
	}
	view.SetReadOnly(true)
	view.SetText(strings.ReplaceAll(text, "\n", "\r\n"))
	close := toolButton(d, "Закрыть", d.Accept)
	d.SetCancelButton(close)
	d.Run()
}
func (dashboard *Dashboard) toolError(err error) {
	if err != nil {
		dashboard.lastError = err.Error()
		_ = dashboard.Invalidate()
	}
}

func (dashboard *Dashboard) showTools() {
	d, err := toolDialog(dashboard.Form(), "Управление Pinus Preview", 600)
	if err != nil {
		dashboard.toolError(err)
		return
	}
	defer d.Dispose()
	actions, err := walk.NewScrollView(d)
	if err != nil {
		return
	}
	actions.SetLayout(walk.NewVBoxLayout())
	hold, err := walk.NewCheckBox(actions)
	if err != nil {
		return
	}
	hold.SetText("Фиксировать подключение — применять изменения только по кнопке")
	hold.SetChecked(dashboard.preferences.HoldConnection)
	hold.CheckedChanged().Attach(func() {
		prefs := smart.UserPreferences{HoldConnection: hold.Checked()}
		if !dashboard.preview {
			if err := smart.SavePreferences(prefs); err != nil {
				dashboard.toolError(err)
				return
			}
		}
		dashboard.preferences = prefs
	})
	toolLabel(actions, "Выбор профиля и правки сохраняются отдельно от работающего подключения.")
	var next func()
	add := func(label string, action func()) { toolButton(actions, label, func() { next = action; d.Accept() }) }
	add("Найти профиль или правило (Ctrl+F)", dashboard.searchDialog)
	add("Импортировать одноразовую ссылку из Pinus-бота", dashboard.importLinkDialog)
	add("Защита при сбое и резервные профили", dashboard.recoveryOptionsDialog)
	add("Отключить VPN и снять блокировку Preview", dashboard.stopConnection)
	add("Применить настройки выбранного профиля (с переподключением)", dashboard.applyPending)
	add("Применённые настройки активного подключения", dashboard.showAppliedSettings)
	add("Проверить handshake / HTTPS через VPN", dashboard.probeConnection)
	add("Почему приложение или сайт идёт этим маршрутом?", dashboard.explainRouteDialog)
	add("Состав VK и «Белых списков»", func() { showToolText(dashboard.Form(), "Состав сервисов", smart.RegionalCatalogText()) })
	add("История маршрутизации и восстановление", dashboard.restoreHistoryDialog)
	add("Создать зашифрованную резервную копию", dashboard.exportEncryptedBackup)
	add("Восстановить резервную копию в новые профили", dashboard.restoreEncryptedBackup)
	add("Дублировать выбранный профиль", dashboard.duplicateSelectedProfile)
	add("Применить последнюю проверенную версию пресетов", func() {
		settings, err := smart.CaptureCatalog(dashboard.settings)
		if err != nil {
			dashboard.toolError(err)
			return
		}
		dashboard.saveSettings(settings, true)
	})
	close := toolButton(d, "Закрыть", d.Accept)
	d.SetCancelButton(close)
	d.Run()
	if next != nil {
		next()
	}
}

func (dashboard *Dashboard) applyPending() {
	if dashboard.selectedProfile() == nil || dashboard.operationBusy() {
		return
	}
	if !dashboard.preview {
		config, err := smart.LoadPendingProfile(dashboard.selectedProfile().Tunnel.Name)
		if err != nil {
			dashboard.toolError(err)
			return
		}
		if config != nil {
			dashboard.applyProfileEdit(dashboard.selected, config, true)
			return
		}
	}
	if shouldDisconnectForRoutingSettings(dashboard.settings) {
		dashboard.stopConnection()
		return
	}
	dashboard.startSelectedProfile()
}

func (dashboard *Dashboard) explainRouteDialog() {
	d, err := toolDialog(dashboard.Form(), "Проверить ожидаемый маршрут", 360)
	if err != nil {
		return
	}
	defer d.Dispose()
	toolLabel(d, "Расчёт по сохранённым настройкам выбранного профиля. Достаточно одного поля.")
	toolLabel(d, "Домен или URL")
	domain, _ := walk.NewLineEdit(d)
	toolLabel(d, "Приложение: EXE или полный путь")
	app, _ := walk.NewLineEdit(d)
	toolLabel(d, "IP-адрес, если известен")
	ip, _ := walk.NewLineEdit(d)
	if domain == nil || app == nil || ip == nil {
		return
	}
	check := toolButton(d, "Объяснить", func() {
		result, err := smart.ExplainRoute(dashboard.settings, smart.RouteQuery{Application: app.Text(), Domain: domain.Text(), IP: ip.Text()})
		if err != nil {
			walk.MsgBox(d, "Проверьте запрос", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		showToolText(d, "Результат расчёта", result.String())
	})
	d.SetDefaultButton(check)
	d.SetCancelButton(toolButton(d, "Закрыть", d.Accept))
	d.Run()
}

func (dashboard *Dashboard) restoreHistoryDialog() {
	p := dashboard.selectedProfile()
	if p == nil || dashboard.preview {
		return
	}
	history, err := smart.LoadSettingsHistory(p.Tunnel.Name)
	if err != nil {
		dashboard.toolError(err)
		return
	}
	if len(history) == 0 {
		showToolText(dashboard.Form(), "История", "Предыдущих настроек ещё нет.")
		return
	}
	d, err := toolDialog(dashboard.Form(), "История маршрутизации", 260)
	if err != nil {
		return
	}
	defer d.Dispose()
	labels := make([]string, len(history))
	for i, r := range history {
		labels[i] = r.At.Local().Format("02.01.2006 15:04:05") + " · " + string(r.Settings.Mode)
	}
	toolLabel(d, "Хранятся 12 предыдущих вариантов. Восстановление подчиняется фиксации подключения.")
	choice, _ := walk.NewComboBox(d)
	if choice == nil {
		return
	}
	choice.SetModel(labels)
	choice.SetCurrentIndex(len(labels) - 1)
	toolButton(d, "Посмотреть изменения", func() {
		i := choice.CurrentIndex()
		if i < 0 {
			return
		}
		s := history[i].Settings
		text := fmt.Sprintf("Режим: %s\nVK через VPN: %t\nРоссийские сервисы через VPN: %t\nПользовательских правил: %d", s.Mode, s.ServiceUsesVPN(smart.VKServiceID), s.ServiceUsesVPN(smart.RussianServiceID), len(s.CustomRules))
		showToolText(d, "Выбранная версия", text)
	})
	restore := -1
	toolButton(d, "Восстановить выбранную версию", func() { restore = choice.CurrentIndex(); d.Accept() })
	d.SetCancelButton(toolButton(d, "Отмена", d.Cancel))
	d.Run()
	if restore >= 0 {
		dashboard.saveSettings(history[restore].Settings, true)
	}
}

func backupPassword(owner walk.Form, confirm bool) (string, bool) {
	d, err := toolDialog(owner, "Пароль резервной копии", 260)
	if err != nil {
		return "", false
	}
	defer d.Dispose()
	toolLabel(d, "Минимум 12 символов. Без пароля восстановить копию невозможно.")
	password, _ := walk.NewLineEdit(d)
	if password == nil {
		return "", false
	}
	password.SetPasswordMode(true)
	var repeat *walk.LineEdit
	if confirm {
		toolLabel(d, "Повторите пароль")
		repeat, _ = walk.NewLineEdit(d)
		if repeat == nil {
			return "", false
		}
		repeat.SetPasswordMode(true)
	}
	accepted := false
	ok := toolButton(d, "Продолжить", func() {
		if confirm && (len([]rune(password.Text())) < 12 || password.Text() != repeat.Text()) {
			walk.MsgBox(d, "Пароль", "Пароли должны совпадать и содержать хотя бы 12 символов.", walk.MsgBoxIconWarning)
			return
		}
		accepted = true
		d.Accept()
	})
	d.SetDefaultButton(ok)
	d.SetCancelButton(toolButton(d, "Отмена", d.Cancel))
	d.Run()
	value := password.Text()
	password.SetText("")
	if repeat != nil {
		repeat.SetText("")
	}
	return value, accepted
}

func (dashboard *Dashboard) exportEncryptedBackup() {
	if dashboard.preview || dashboard.operationBusy() {
		return
	}
	file := walk.FileDialog{Title: "Сохранить зашифрованную копию", Filter: "Pinus backup (*.pinusbackup)|*.pinusbackup", FilePath: "Pinus-" + time.Now().Format("2006-01-02") + ".pinusbackup"}
	ok, err := file.ShowSave(dashboard.Form())
	if err != nil {
		dashboard.toolError(err)
		return
	}
	if !ok {
		return
	}
	password, ok := backupPassword(dashboard.Form(), true)
	if !ok {
		return
	}
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	go func() {
		backup := smart.Backup{Version: 1, CreatedAt: time.Now().UTC()}
		tunnels, err := manager.IPCClientTunnels()
		for _, tunnel := range tunnels {
			if err != nil {
				break
			}
			var config conf.Config
			config, err = tunnel.StoredConfig()
			if err != nil {
				break
			}
			var settings smart.RoutingSettings
			settings, err = smart.LoadSettings(tunnel.Name)
			if err != nil {
				break
			}
			backup.Profiles = append(backup.Profiles, smart.BackupProfile{Name: tunnel.Name, Config: config.ToWgQuick(), Settings: settings})
		}
		if err == nil {
			var data []byte
			data, err = smart.EncryptBackup(backup, password)
			if err == nil {
				err = smart.WriteBackupFile(file.FilePath, data)
			}
		}
		password = ""
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			if err != nil {
				dashboard.toolError(err)
			} else {
				dashboard.lastError = "Зашифрованная копия сохранена."
				_ = dashboard.Invalidate()
			}
		})
	}()
}

func uniqueRestoredName(base string, used map[string]bool) string {
	if !used[strings.ToLower(base)] {
		used[strings.ToLower(base)] = true
		return base
	}
	runes := []rune(base)
	if len(runes) > 20 {
		base = string(runes[:20])
	}
	for n := 2; ; n++ {
		name := fmt.Sprintf("%s-copy%d", base, n)
		if !used[strings.ToLower(name)] {
			used[strings.ToLower(name)] = true
			return name
		}
	}
}

func (dashboard *Dashboard) restoreEncryptedBackup() {
	if dashboard.preview || dashboard.operationBusy() {
		return
	}
	file := walk.FileDialog{Title: "Восстановить зашифрованную копию", Filter: "Pinus backup (*.pinusbackup)|*.pinusbackup"}
	ok, err := file.ShowOpen(dashboard.Form())
	if err != nil {
		dashboard.toolError(err)
		return
	}
	if !ok {
		return
	}
	password, ok := backupPassword(dashboard.Form(), false)
	if !ok {
		return
	}
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	go func() {
		var backup smart.Backup
		info, err := os.Stat(file.FilePath)
		if err == nil && info.Size() > 17*1024*1024 {
			err = errors.New("файл копии слишком велик")
		}
		if err == nil {
			var data []byte
			data, err = os.ReadFile(file.FilePath)
			if err == nil {
				backup, err = smart.DecryptBackup(data, password)
			}
		}
		password = ""
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			if err != nil {
				dashboard.toolError(err)
				return
			}
			names := []string{}
			for _, p := range backup.Profiles {
				names = append(names, p.Name)
			}
			if walk.MsgBox(dashboard.Form(), "Восстановить копию", fmt.Sprintf("Профилей: %d\n%s\n\nСоздать отдельные профили? Совпадающим именам будет добавлен суффикс. Подключение не переключается.", len(names), strings.Join(names, ", ")), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
				return
			}
			dashboard.restoreBackupProfiles(backup.Profiles)
		})
	}()
}

func (dashboard *Dashboard) restoreBackupProfiles(profiles []smart.BackupProfile) {
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	go func() {
		tunnels, err := manager.IPCClientTunnels()
		used := map[string]bool{}
		for _, t := range tunnels {
			used[strings.ToLower(t.Name)] = true
		}
		rename := map[string]string{}
		for _, p := range profiles {
			rename[p.Name] = uniqueRestoredName(p.Name, used)
		}
		restored := 0
		for _, p := range profiles {
			if err != nil {
				break
			}
			var config *conf.Config
			config, err = conf.FromWgQuick(p.Config, rename[p.Name])
			if err != nil {
				break
			}
			_, err = manager.IPCClientNewTunnel(config)
			if err != nil {
				break
			}
			settings := cloneRoutingSettings(p.Settings)
			for i, name := range settings.FallbackProfiles {
				if replacement, ok := rename[name]; ok {
					settings.FallbackProfiles[i] = replacement
				}
			}
			err = smart.SaveSettings(config.Name, settings)
			restored++
		}
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			dashboard.loadProfiles()
			dashboard.lastError = fmt.Sprintf("Восстановлено профилей: %d из %d.", restored, len(profiles))
			if err != nil {
				dashboard.lastError += " " + err.Error()
			}
			_ = dashboard.Invalidate()
		})
	}()
}
func (dashboard *Dashboard) duplicateSelectedProfile() {
	p := dashboard.selectedProfile()
	if p == nil || dashboard.preview || dashboard.operationBusy() {
		return
	}
	c, err := p.Tunnel.StoredConfig()
	if err != nil {
		dashboard.toolError(err)
		return
	}
	dashboard.restoreBackupProfiles([]smart.BackupProfile{{Name: p.Tunnel.Name, Config: c.ToWgQuick(), Settings: cloneRoutingSettings(dashboard.settings)}})
}

func (dashboard *Dashboard) showAppliedSettings() {
	if dashboard.preview {
		return
	}
	go func() {
		snapshot, err := manager.IPCClientConnectionSnapshot()
		dashboard.Synchronize(func() {
			if err != nil {
				dashboard.toolError(err)
				return
			}
			if snapshot.Name == "" {
				showToolText(dashboard.Form(), "Применённые настройки", "Нет активного подключения.")
				return
			}
			text := fmt.Sprintf("Активный профиль: %s\nДвижок: %s\nРежим: %s\nVK через VPN: %t\nРоссийские сервисы через VPN: %t\nПравил: %d", snapshot.Name, snapshot.Engine, snapshot.Settings.Mode, snapshot.Settings.ServiceUsesVPN(smart.VKServiceID), snapshot.Settings.ServiceUsesVPN(smart.RussianServiceID), len(snapshot.Settings.CustomRules))
			showToolText(dashboard.Form(), "Применённые настройки", text)
		})
	}()
}
func (dashboard *Dashboard) probeConnection() {
	if dashboard.preview || !atomic.CompareAndSwapUint32(&dashboard.diagnosticsBusy, 0, 1) {
		return
	}
	dashboard.lastError = "Проверяю соединение (до 24 секунд)..."
	_ = dashboard.Invalidate()
	go func() {
		result, err := manager.IPCClientProbeConnection()
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.diagnosticsBusy, 0)
			dashboard.lastError = ""
			if err != nil {
				dashboard.toolError(err)
			} else {
				showToolText(dashboard.Form(), "Проверка соединения", result)
			}
			_ = dashboard.Invalidate()
		})
	}()
}
