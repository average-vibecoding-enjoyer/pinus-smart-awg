package ui

import (
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/lxn/walk"
	"strings"
)

func (dashboard *Dashboard) recoveryOptionsDialog() {
	p := dashboard.selectedProfile()
	if p == nil {
		return
	}
	d, err := toolDialog(dashboard.Form(), "Восстановление и защита", 450)
	if err != nil {
		return
	}
	defer d.Dispose()
	protection, _ := walk.NewCheckBox(d)
	if protection == nil {
		return
	}
	protection.SetText("При сбое блокировать интернет до восстановления или явного отключения")
	protection.SetChecked(dashboard.settings.Protection == "all")
	toolLabel(d, "Защита распространяется и на прямые исключения, когда движок остановлен.")
	toolLabel(d, "Preview: фильтры сохраняются после сбоя менеджера; работа до запуска BFE не покрывается.")
	toolLabel(d, "Снять блокировку: Управление → Отключить VPN и снять блокировку Preview.")
	toolLabel(d, "Резервные профили по порядку, по одному имени в строке (до 8):")
	fallback, _ := walk.NewTextEdit(d)
	if fallback == nil {
		return
	}
	fallback.SetText(strings.Join(dashboard.settings.FallbackProfiles, "\r\n"))
	toolLabel(d, "Резервы пробуются при ошибке запуска/восстановления движка. Доступность сервера сама по себе не проверяется автоматически.")
	var settings smart.RoutingSettings
	accepted := false
	save := toolButton(d, "Сохранить", func() {
		settings = cloneRoutingSettings(dashboard.settings)
		settings.Protection = ""
		if protection.Checked() {
			settings.Protection = "all"
		}
		settings.FallbackProfiles = nil
		seen := map[string]bool{}
		for _, raw := range strings.Split(fallback.Text(), "\n") {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			valid := false
			for _, candidate := range dashboard.profiles {
				if candidate.Tunnel.Name == name && name != p.Tunnel.Name {
					valid = true
				}
			}
			if !valid {
				walk.MsgBox(d, "Резервный профиль", "Укажи существующий профиль, отличный от текущего: "+name, walk.MsgBoxIconWarning)
				return
			}
			if !seen[name] {
				settings.FallbackProfiles = append(settings.FallbackProfiles, name)
				seen[name] = true
			}
		}
		if _, err := settings.Normalized(); err != nil {
			walk.MsgBox(d, "Настройки", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		accepted = true
		d.Accept()
	})
	d.SetDefaultButton(save)
	d.SetCancelButton(toolButton(d, "Отмена", d.Cancel))
	d.Run()
	if accepted {
		dashboard.saveSettings(settings, true)
	}
}
