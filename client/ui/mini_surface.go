/* SPDX-License-Identifier: MIT */

package ui

import (
	"errors"
	"fmt"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

type miniSurface struct {
	*walk.CustomWidget

	theme         *dashboardTheme
	smooth        *smoothCanvas
	backing       *walk.Bitmap
	backingCanvas *walk.Canvas
	backingSize   walk.Size
	backingDPI    int
	gdiCommands   []dashboardGDICommand
	hits          []dashboardHit
	hoverID       string
	pressedID     string
	paintContent  func(*miniSurface, *walk.Canvas, walk.Rectangle)
	activate      func(string)
	wheel         func(int)
}

func newMiniSurface(parent walk.Container, paintContent func(*miniSurface, *walk.Canvas, walk.Rectangle), activate func(string)) (*miniSurface, error) {
	theme, err := newDashboardTheme()
	if err != nil {
		return nil, err
	}
	surface := &miniSurface{theme: theme, paintContent: paintContent, activate: activate}
	widget, err := walk.NewCustomWidgetPixels(parent, win.WS_TABSTOP, surface.paint)
	if err != nil {
		theme.Dispose()
		return nil, err
	}
	surface.CustomWidget = widget
	surface.SetPaintMode(walk.PaintNoErase)
	surface.SetInvalidatesOnResize(false)
	surface.SetCursor(walk.CursorArrow())
	surface.SetToolTipText("")
	surface.MouseMove().Attach(surface.onMouseMove)
	surface.MouseDown().Attach(surface.onMouseDown)
	surface.MouseUp().Attach(surface.onMouseUp)
	surface.MouseWheel().Attach(func(_, _ int, button walk.MouseButton) {
		if surface.wheel != nil {
			surface.wheel(walk.MouseWheelEventDelta(button))
		}
	})
	surface.SizeChanged().Attach(func() { _ = surface.Invalidate() })
	surface.Disposing().Attach(surface.disposeResources)
	return surface, nil
}

func installMiniDialogLayout(dialog *walk.Dialog) error {
	layout := walk.NewHBoxLayout()
	if err := layout.SetMargins(walk.Margins{}); err != nil {
		return err
	}
	if err := layout.SetSpacing(0); err != nil {
		return err
	}
	return dialog.SetLayout(layout)
}

// Keep native edit controls above the painted surface without letting Walk's
// mandatory form layout reposition them.
func detachMiniOverlayWidget(parent walk.Container, widget walk.Widget) error {
	if parent == nil || widget == nil {
		return errors.New("mini dialog overlay requires a parent and widget")
	}
	container := parent.AsContainerBase()
	if container == nil {
		return errors.New("mini dialog overlay requires a native container")
	}
	if err := parent.Children().Remove(widget); err != nil {
		return err
	}

	hwnd := widget.Handle()
	style := uint32(win.GetWindowLong(hwnd, win.GWL_STYLE))
	style |= win.WS_CHILD
	style &^= win.WS_POPUP
	win.SetWindowLong(hwnd, win.GWL_STYLE, int32(style))
	win.SetWindowLong(hwnd, win.GWL_ID, container.NextChildID())
	win.SetParent(hwnd, container.Handle())
	if win.GetParent(hwnd) != container.Handle() {
		widget.Dispose()
		return errors.New("reattach mini dialog overlay")
	}
	if !win.SetWindowPos(hwnd, win.HWND_TOP, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_FRAMECHANGED|win.SWP_SHOWWINDOW) {
		widget.Dispose()
		return errors.New("position mini dialog overlay")
	}
	return nil
}

func (surface *miniSurface) disposeResources() {
	if surface.smooth != nil {
		surface.smooth.Dispose()
		surface.smooth = nil
	}
	if surface.backingCanvas != nil {
		surface.backingCanvas.Dispose()
		surface.backingCanvas = nil
	}
	if surface.backing != nil {
		surface.backing.Dispose()
		surface.backing = nil
	}
	if surface.theme != nil {
		surface.theme.Dispose()
		surface.theme = nil
	}
}

func (surface *miniSurface) Invalidate() error {
	if surface.CustomWidget == nil || surface.Handle() == 0 {
		return nil
	}
	if !win.InvalidateRect(surface.Handle(), nil, false) {
		return errors.New("invalidate mini dialog")
	}
	return nil
}

func (surface *miniSurface) px(value int) int {
	return surface.IntFrom96DPI(value)
}

func (surface *miniSurface) addHit(id string, bounds walk.Rectangle, disabled bool) {
	if id == "" {
		return
	}
	surface.hits = append(surface.hits, dashboardHit{ID: id, Bounds: bounds, Disabled: disabled})
}

func (surface *miniSurface) hitAt(x, y int) dashboardHit {
	for index := len(surface.hits) - 1; index >= 0; index-- {
		if pointInside(x, y, surface.hits[index].Bounds) {
			return surface.hits[index]
		}
	}
	return dashboardHit{}
}

func (surface *miniSurface) onMouseMove(x, y int, _ walk.MouseButton) {
	hit := surface.hitAt(x, y)
	if hit.ID == surface.hoverID {
		return
	}
	surface.hoverID = hit.ID
	if hit.ID != "" && !hit.Disabled {
		surface.SetCursor(walk.CursorHand())
	} else {
		surface.SetCursor(walk.CursorArrow())
	}
	_ = surface.Invalidate()
}

func (surface *miniSurface) onMouseDown(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	hit := surface.hitAt(x, y)
	if hit.Disabled {
		return
	}
	surface.pressedID = hit.ID
	_ = surface.SetFocus()
	_ = surface.Invalidate()
}

func (surface *miniSurface) onMouseUp(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	hit := surface.hitAt(x, y)
	pressed := surface.pressedID
	surface.pressedID = ""
	_ = surface.Invalidate()
	if pressed != "" && pressed == hit.ID && !hit.Disabled && surface.activate != nil {
		surface.activate(pressed)
	}
}

func (surface *miniSurface) ensureBacking(size walk.Size) error {
	dpi := surface.DPI()
	if surface.backing != nil && surface.backingCanvas != nil && surface.backingSize == size && surface.backingDPI == dpi {
		return nil
	}
	if surface.backingCanvas != nil {
		surface.backingCanvas.Dispose()
		surface.backingCanvas = nil
	}
	if surface.backing != nil {
		surface.backing.Dispose()
		surface.backing = nil
	}
	bitmap, err := walk.NewBitmapForDPI(size, dpi)
	if err != nil {
		return err
	}
	canvas, err := walk.NewCanvasFromImage(bitmap)
	if err != nil {
		bitmap.Dispose()
		return err
	}
	surface.backing = bitmap
	surface.backingCanvas = canvas
	surface.backingSize = size
	surface.backingDPI = dpi
	return nil
}

func (surface *miniSurface) paint(canvas *walk.Canvas, _ walk.Rectangle) error {
	if surface.theme == nil {
		return nil
	}
	bounds := surface.ClientBoundsPixels()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return nil
	}
	if err := surface.ensureBacking(walk.Size{Width: bounds.Width, Height: bounds.Height}); err != nil {
		return err
	}
	surface.hits = surface.hits[:0]
	surface.gdiCommands = surface.gdiCommands[:0]
	surface.smooth = newSmoothCanvas(surface.backingCanvas.HDC())
	surface.fillGradient(surface.backingCanvas, bounds, surface.theme.backgroundTop, surface.theme.backgroundEnd, 2, surface.theme.background)
	if surface.paintContent != nil {
		surface.paintContent(surface, surface.backingCanvas, bounds)
	}
	if surface.smooth != nil {
		surface.smooth.Dispose()
		surface.smooth = nil
	}
	surface.flushGDI(surface.backingCanvas)
	win.GdiFlush()
	if !win.BitBlt(canvas.HDC(), 0, 0, int32(bounds.Width), int32(bounds.Height), surface.backingCanvas.HDC(), 0, 0, win.SRCCOPY) {
		return fmt.Errorf("copy mini dialog back buffer")
	}
	return nil
}

func (surface *miniSurface) drawText(canvas *walk.Canvas, text string, font *walk.Font, color walk.Color, bounds walk.Rectangle, format walk.DrawTextFormat) {
	surface.gdiCommands = append(surface.gdiCommands, dashboardGDICommand{text: text, font: font, color: color, bounds: bounds, format: format})
}

func (surface *miniSurface) drawImage(_ *walk.Canvas, image walk.Image, bounds walk.Rectangle) {
	if image != nil {
		surface.gdiCommands = append(surface.gdiCommands, dashboardGDICommand{image: image, bounds: bounds})
	}
}

func (surface *miniSurface) flushGDI(canvas *walk.Canvas) {
	for _, command := range surface.gdiCommands {
		if command.image != nil {
			_ = canvas.DrawImageStretchedPixels(command.image, command.bounds)
			continue
		}
		_ = canvas.DrawTextPixels(command.text, command.font, command.color, command.bounds, command.format|walk.TextNoPrefix)
	}
	surface.gdiCommands = surface.gdiCommands[:0]
}

func (surface *miniSurface) fillGradient(canvas *walk.Canvas, bounds walk.Rectangle, start, end walk.Color, direction int, fallback *walk.SolidColorBrush) {
	if surface.smooth != nil && surface.smooth.FillRectangleGradient(bounds, start, end, direction) {
		return
	}
	_ = canvas.FillRectanglePixels(fallback, bounds)
}

func (surface *miniSurface) fillRounded(canvas *walk.Canvas, color walk.Color, fallback *walk.SolidColorBrush, bounds walk.Rectangle, radius int) {
	if surface.smooth != nil && surface.smooth.FillRoundedRectangle(color, bounds, radius) {
		return
	}
	_ = canvas.FillRoundedRectanglePixels(fallback, bounds, walk.Size{Width: radius, Height: radius})
}

func (surface *miniSurface) fillRoundedGradient(canvas *walk.Canvas, bounds walk.Rectangle, radius int, start, end walk.Color, fallback *walk.SolidColorBrush) {
	if surface.smooth != nil && surface.smooth.FillRoundedGradient(bounds, radius, start, end, 2) {
		return
	}
	surface.fillRounded(canvas, fallback.Color(), fallback, bounds, radius)
}

func (surface *miniSurface) borderedCard(canvas *walk.Canvas, bounds walk.Rectangle, fill *walk.SolidColorBrush, border walk.Color, radius, width int) {
	if surface.smooth != nil && surface.smooth.FillRoundedRectangle(border, bounds, radius) {
		inner := insetRectangle(bounds, width)
		if surface.smooth.FillRoundedRectangle(fill.Color(), inner, radius-width) {
			return
		}
	}
	surface.fillRounded(canvas, fill.Color(), fill, bounds, radius)
}

func (surface *miniSurface) interactiveBrush(id string, selected bool) *walk.SolidColorBrush {
	if selected {
		return surface.theme.surfaceSelected
	}
	if id != "" && (surface.hoverID == id || surface.pressedID == id) {
		return surface.theme.surfaceHover
	}
	return surface.theme.surface
}

func (surface *miniSurface) drawCard(canvas *walk.Canvas, id string, bounds walk.Rectangle, selected, disabled bool) {
	fill := surface.interactiveBrush(id, selected)
	if disabled {
		fill = surface.theme.sidebar
	}
	border := surface.theme.border.Color()
	width := surface.px(1)
	if selected {
		border = surface.theme.accentColor
		width = surface.px(2)
	}
	if selected && !disabled {
		if surface.smooth != nil && surface.smooth.FillRoundedRectangle(border, bounds, surface.px(8)) {
			inner := insetRectangle(bounds, width)
			if surface.smooth.FillRoundedGradient(inner, surface.px(7), surface.theme.selectedTop, surface.theme.selectedEnd, 2) {
				surface.addHit(id, bounds, disabled)
				return
			}
		}
	}
	surface.borderedCard(canvas, bounds, fill, border, surface.px(8), width)
	surface.addHit(id, bounds, disabled)
}

func (surface *miniSurface) drawButton(canvas *walk.Canvas, id, label, icon string, bounds walk.Rectangle, primary, disabled bool) {
	fill := surface.interactiveBrush(id, false)
	if primary {
		fill = surface.theme.accentSoft
		if surface.hoverID == id || surface.pressedID == id {
			fill = surface.theme.accent
		}
	}
	if disabled {
		fill = surface.theme.sidebar
	}
	if primary {
		surface.fillRoundedGradient(canvas, bounds, surface.px(7), surface.theme.selectedTop, fill.Color(), fill)
	} else {
		surface.borderedCard(canvas, bounds, fill, surface.theme.border.Color(), surface.px(7), surface.px(1))
	}
	color := surface.theme.textColor
	if disabled {
		color = surface.theme.faintColor
	}
	iconWidth := 0
	if icon != "" {
		iconWidth = surface.px(24)
		surface.drawText(canvas, icon, surface.theme.iconFont, color, walk.Rectangle{X: bounds.X + surface.px(12), Y: bounds.Y, Width: iconWidth, Height: bounds.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	}
	textX := bounds.X + surface.px(14) + iconWidth
	surface.drawText(canvas, label, surface.theme.smallFont, color, walk.Rectangle{X: textX, Y: bounds.Y, Width: bounds.X + bounds.Width - textX - surface.px(12), Height: bounds.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	surface.addHit(id, bounds, disabled)
}

func (surface *miniSurface) drawSegment(canvas *walk.Canvas, id, label, detail string, bounds walk.Rectangle, selected, disabled bool) {
	surface.drawCard(canvas, id, bounds, selected, disabled)
	color := surface.theme.textColor
	muted := surface.theme.mutedColor
	if disabled {
		color = surface.theme.faintColor
		muted = surface.theme.faintColor
	}
	dotColor := surface.theme.faintColor
	if selected {
		dotColor = surface.theme.accentColor
	}
	dot := walk.Rectangle{X: bounds.X + surface.px(14), Y: bounds.Y + surface.px(16), Width: surface.px(10), Height: surface.px(10)}
	if surface.smooth != nil {
		_ = surface.smooth.FillEllipse(dotColor, dot)
	}
	surface.drawText(canvas, label, surface.theme.headingFont, color, walk.Rectangle{X: bounds.X + surface.px(34), Y: bounds.Y + surface.px(8), Width: bounds.Width - surface.px(46), Height: surface.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	if detail != "" {
		surface.drawText(canvas, detail, surface.theme.smallFont, muted, walk.Rectangle{X: bounds.X + surface.px(14), Y: bounds.Y + surface.px(31), Width: bounds.Width - surface.px(28), Height: bounds.Height - surface.px(36)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	}
}

func (surface *miniSurface) drawToggle(canvas *walk.Canvas, id string, bounds walk.Rectangle, checked, disabled bool) {
	track := surface.theme.faint
	if checked {
		track = surface.theme.accent
	}
	if disabled {
		track = surface.theme.sidebar
	}
	surface.fillRounded(canvas, track.Color(), track, bounds, bounds.Height/2)
	knobSize := bounds.Height - surface.px(6)
	knobX := bounds.X + surface.px(3)
	if checked {
		knobX = bounds.X + bounds.Width - knobSize - surface.px(3)
	}
	knobBounds := walk.Rectangle{X: knobX, Y: bounds.Y + surface.px(3), Width: knobSize, Height: knobSize}
	if surface.smooth != nil {
		_ = surface.smooth.FillEllipse(surface.theme.textColor, knobBounds)
	}
	surface.addHit(id, bounds, disabled)
}

func (surface *miniSurface) drawFieldFrame(canvas *walk.Canvas, id string, bounds walk.Rectangle, focused, invalid bool) {
	border := surface.theme.border.Color()
	if focused {
		border = surface.theme.accentColor
	}
	if invalid {
		border = surface.theme.dangerColor
	}
	surface.borderedCard(canvas, bounds, surface.theme.surface, border, surface.px(7), surface.px(2))
	surface.addHit(id, bounds, false)
}

func (surface *miniSurface) drawPill(canvas *walk.Canvas, text string, bounds walk.Rectangle, selected bool) {
	fill := surface.theme.sidebar
	color := surface.theme.mutedColor
	if selected {
		fill = surface.theme.accentSoft
		color = surface.theme.textColor
	}
	surface.fillRounded(canvas, fill.Color(), fill, bounds, bounds.Height/2)
	surface.drawText(canvas, text, surface.theme.microFont, color, bounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
}

func styleMiniLineEdit(edit *walk.LineEdit, surface *miniSurface) {
	if edit == nil || surface == nil || surface.theme == nil {
		return
	}
	win.SetWindowLong(edit.Handle(), win.GWL_EXSTYLE, win.GetWindowLong(edit.Handle(), win.GWL_EXSTYLE)&^win.WS_EX_CLIENTEDGE)
	win.SetWindowPos(edit.Handle(), 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
	edit.SetTextColor(surface.theme.textColor)
	edit.SetBackground(surface.theme.surface)
	edit.SetFont(surface.theme.bodyFont)
	_ = win.SetWindowTheme(edit.Handle(), windows.StringToUTF16Ptr("DarkMode_Explorer"), nil)
	edit.FocusedChanged().Attach(func() { _ = surface.Invalidate() })
}

func placeMiniLineEdit(edit *walk.LineEdit, frame walk.Rectangle, surface *miniSurface) {
	if edit == nil || surface == nil {
		return
	}
	insetX := surface.px(10)
	insetY := surface.px(7)
	_ = edit.SetBoundsPixels(walk.Rectangle{
		X:      frame.X + insetX,
		Y:      frame.Y + insetY,
		Width:  frame.Width - insetX*2,
		Height: frame.Height - insetY*2,
	})
}
