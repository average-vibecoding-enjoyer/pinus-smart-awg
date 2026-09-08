package ui

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func (dashboard *Dashboard) WndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == win.WM_GETDLGCODE {
		return win.DLGC_WANTALLKEYS | win.DLGC_WANTTAB
	}
	return dashboard.CustomWidget.WndProc(hwnd, msg, wParam, lParam)
}
func keyboardActions(hits []dashboardHit) []string {
	seen := map[string]bool{}
	var ids []string
	for _, hit := range hits {
		if !hit.Disabled && !seen[hit.ID] {
			seen[hit.ID] = true
			ids = append(ids, hit.ID)
		}
	}
	return ids
}
func (dashboard *Dashboard) focusNext(reverse bool) {
	ids := keyboardActions(dashboard.hits)
	if len(ids) == 0 {
		return
	}
	index := -1
	for i, id := range ids {
		if id == dashboard.focusID {
			index = i
			break
		}
	}
	if reverse {
		if index < 0 {
			index = 0
		}
		index = (index + len(ids) - 1) % len(ids)
	} else {
		index = (index + 1) % len(ids)
	}
	dashboard.focusID = ids[index]
	dashboard.SetToolTipText(ids[index])
	_ = dashboard.Invalidate()
}
func (dashboard *Dashboard) drawKeyboardFocus(canvas *walk.Canvas) {
	if dashboard.focusID == "" {
		return
	}
	pen, err := walk.NewCosmeticPen(walk.PenDash, dashboard.theme.accent.Color())
	if err != nil {
		return
	}
	defer pen.Dispose()
	for _, hit := range dashboard.hits {
		if !hit.Disabled && hit.ID == dashboard.focusID {
			canvas.DrawRectangle(pen, insetRectangle(hit.Bounds, 3))
			break
		}
	}
}
