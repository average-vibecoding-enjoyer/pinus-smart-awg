package ui

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/lxn/walk"
)

func (dashboard *Dashboard) importLinkDialog() {
	if dashboard.preview || dashboard.operationBusy() {
		return
	}
	d, err := toolDialog(dashboard.Form(), "Одноразовая ссылка из Pinus-бота", 260)
	if err != nil {
		return
	}
	defer d.Dispose()
	toolLabel(d, "Вставьте ссылку от своего Pinus-бота. Получение погасит её; профиль добавится после просмотра.")
	entry, err := walk.NewLineEdit(d)
	if err != nil {
		return
	}
	entry.SetPasswordMode(true)
	var link smart.ImportLink
	accepted := false
	button := toolButton(d, "Получить профиль", func() {
		parsed, err := smart.ParseImportLink(entry.Text())
		if err != nil {
			walk.MsgBox(d, "Проверьте ссылку", err.Error(), walk.MsgBoxIconWarning)
			return
		}
		if walk.MsgBox(d, "Сервер импорта", "Получить профиль с "+parsed.Issuer+"?\nПродолжайте, если это сервер вашего Pinus-бота.", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			return
		}
		link = parsed
		accepted = true
		entry.SetText("")
		d.Accept()
	})
	d.SetDefaultButton(button)
	d.SetCancelButton(toolButton(d, "Отмена", d.Cancel))
	d.Run()
	if !accepted || !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(dashboard.presetContext, 15*time.Second)
		defer cancel()
		config, err := smart.ClaimImportLink(ctx, link)
		if dashboard.presetContext.Err() != nil {
			return
		}
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			if err != nil {
				dashboard.toolError(err)
				return
			}
			message := fmt.Sprintf("Профиль: %s\nПротокол: %s\nПиров: %d\n\nДобавить отдельный профиль? Подключение останется прежним.\nСсылка уже использована.", config.Name, config.ProtocolDescription(), len(config.Peers))
			if walk.MsgBox(dashboard.Form(), "Добавить профиль", message, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
				return
			}
			dashboard.restoreBackupProfiles([]smart.BackupProfile{{Name: config.Name, Config: config.ToWgQuick(), Settings: smart.DefaultSettings()}})
		})
	}()
}
