package ui

import (
	"github.com/lxn/walk"
	"strings"
)

type searchItem struct {
	text          string
	profile, rule int
}

func (dashboard *Dashboard) searchDialog() {
	if dashboard.operationBusy() {
		return
	}
	d, err := toolDialog(dashboard.Form(), "Найти профиль или правило", 470)
	if err != nil {
		return
	}
	defer d.Dispose()
	toolLabel(d, "Профили и правила выбранного профиля. Поиск по имени, адресу или значению.")
	query, err := walk.NewLineEdit(d)
	if err != nil {
		return
	}
	list, err := walk.NewListBox(d)
	if err != nil {
		return
	}
	items := []searchItem{}
	for i, p := range dashboard.profiles {
		items = append(items, searchItem{text: "Профиль · " + p.Tunnel.Name + " · " + p.Endpoint, profile: i, rule: -1})
	}
	for i, r := range dashboard.settings.CustomRules {
		items = append(items, searchItem{text: "Правило · " + r.Name + " · " + r.Value, profile: -1, rule: i})
	}
	filtered := []searchItem{}
	update := func() {
		filtered = nil
		labels := []string{}
		needle := strings.ToLower(strings.TrimSpace(query.Text()))
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.text), needle) {
				filtered = append(filtered, item)
				labels = append(labels, item.text)
			}
		}
		list.SetModel(labels)
		if len(labels) > 0 {
			list.SetCurrentIndex(0)
		}
	}
	query.TextChanged().Attach(update)
	update()
	var chosen *searchItem
	choose := func() {
		i := list.CurrentIndex()
		if i >= 0 && i < len(filtered) {
			item := filtered[i]
			chosen = &item
			d.Accept()
		}
	}
	list.ItemActivated().Attach(choose)
	d.SetDefaultButton(toolButton(d, "Открыть", choose))
	d.SetCancelButton(toolButton(d, "Отмена", d.Cancel))
	d.Run()
	if chosen == nil {
		return
	}
	if chosen.profile >= 0 {
		dashboard.selectProfile(chosen.profile)
		dashboard.page = pageProfiles
		dashboard.profileScroll = chosen.profile
		_ = dashboard.Invalidate()
	} else {
		dashboard.page = pageRules
		dashboard.ruleScroll = chosen.rule
		dashboard.editRule(chosen.rule)
	}
}
