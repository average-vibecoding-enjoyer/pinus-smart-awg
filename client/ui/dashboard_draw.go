/* SPDX-License-Identifier: MIT */

package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"

	"github.com/amnezia-vpn/amneziawg-windows-client/manager"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
)

type dashboardGDICommand struct {
	image  walk.Image
	text   string
	font   *walk.Font
	color  walk.Color
	bounds walk.Rectangle
	format walk.DrawTextFormat
}

type dashboardTheme struct {
	background      *walk.SolidColorBrush
	sidebar         *walk.SolidColorBrush
	surface         *walk.SolidColorBrush
	surfaceHover    *walk.SolidColorBrush
	surfaceSelected *walk.SolidColorBrush
	accentSoft      *walk.SolidColorBrush
	accent          *walk.SolidColorBrush
	success         *walk.SolidColorBrush
	danger          *walk.SolidColorBrush
	warning         *walk.SolidColorBrush
	faint           *walk.SolidColorBrush
	border          *walk.SolidColorBrush

	borderPen  *walk.GeometricPen
	accentPen  *walk.GeometricPen
	successPen *walk.GeometricPen
	dangerPen  *walk.GeometricPen
	mutedPen   *walk.GeometricPen
	textPen    *walk.GeometricPen

	brandFont   *walk.Font
	titleFont   *walk.Font
	headingFont *walk.Font
	bodyFont    *walk.Font
	smallFont   *walk.Font
	microFont   *walk.Font
	iconFont    *walk.Font
	powerFont   *walk.Font

	textColor     walk.Color
	mutedColor    walk.Color
	faintColor    walk.Color
	accentColor   walk.Color
	successColor  walk.Color
	dangerColor   walk.Color
	warningColor  walk.Color
	backgroundTop walk.Color
	backgroundEnd walk.Color
	sidebarTop    walk.Color
	sidebarEnd    walk.Color
	selectedTop   walk.Color
	selectedEnd   walk.Color
	powerTop      walk.Color
	powerEnd      walk.Color
}

func newDashboardTheme() (*dashboardTheme, error) {
	theme := &dashboardTheme{
		textColor:     walk.RGB(243, 247, 252),
		mutedColor:    walk.RGB(158, 181, 205),
		faintColor:    walk.RGB(93, 122, 151),
		accentColor:   walk.RGB(54, 169, 255),
		successColor:  walk.RGB(46, 210, 139),
		dangerColor:   walk.RGB(245, 92, 113),
		warningColor:  walk.RGB(240, 182, 76),
		backgroundTop: walk.RGB(5, 14, 29),
		backgroundEnd: walk.RGB(9, 27, 48),
		sidebarTop:    walk.RGB(10, 33, 58),
		sidebarEnd:    walk.RGB(7, 22, 40),
		selectedTop:   walk.RGB(20, 78, 126),
		selectedEnd:   walk.RGB(15, 53, 91),
		powerTop:      walk.RGB(23, 75, 122),
		powerEnd:      walk.RGB(14, 40, 70),
	}
	var err error
	makeBrush := func(color walk.Color) (*walk.SolidColorBrush, error) {
		return walk.NewSolidColorBrush(color)
	}
	if theme.background, err = makeBrush(walk.RGB(7, 18, 34)); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			theme.Dispose()
		}
	}()
	if theme.sidebar, err = makeBrush(walk.RGB(9, 27, 50)); err != nil {
		return nil, err
	}
	if theme.surface, err = makeBrush(walk.RGB(15, 40, 70)); err != nil {
		return nil, err
	}
	if theme.surfaceHover, err = makeBrush(walk.RGB(20, 50, 86)); err != nil {
		return nil, err
	}
	if theme.surfaceSelected, err = makeBrush(walk.RGB(17, 60, 101)); err != nil {
		return nil, err
	}
	if theme.accentSoft, err = makeBrush(walk.RGB(13, 74, 119)); err != nil {
		return nil, err
	}
	if theme.accent, err = makeBrush(theme.accentColor); err != nil {
		return nil, err
	}
	if theme.success, err = makeBrush(theme.successColor); err != nil {
		return nil, err
	}
	if theme.danger, err = makeBrush(theme.dangerColor); err != nil {
		return nil, err
	}
	if theme.warning, err = makeBrush(theme.warningColor); err != nil {
		return nil, err
	}
	if theme.faint, err = makeBrush(walk.RGB(42, 69, 96)); err != nil {
		return nil, err
	}
	if theme.border, err = makeBrush(walk.RGB(34, 67, 99)); err != nil {
		return nil, err
	}
	borderBrush := theme.border
	var brushErr error
	mutedBrush, brushErr := makeBrush(theme.faintColor)
	if brushErr != nil {
		return nil, brushErr
	}
	defer mutedBrush.Dispose()
	textBrush, brushErr := makeBrush(theme.textColor)
	if brushErr != nil {
		return nil, brushErr
	}
	defer textBrush.Dispose()
	if theme.borderPen, err = walk.NewGeometricPen(walk.PenSolid|walk.PenJoinRound, 1, borderBrush); err != nil {
		return nil, err
	}
	if theme.accentPen, err = walk.NewGeometricPen(walk.PenSolid|walk.PenCapRound, 3, theme.accent); err != nil {
		return nil, err
	}
	if theme.successPen, err = walk.NewGeometricPen(walk.PenSolid|walk.PenCapRound, 3, theme.success); err != nil {
		return nil, err
	}
	if theme.dangerPen, err = walk.NewGeometricPen(walk.PenSolid|walk.PenCapRound, 2, theme.danger); err != nil {
		return nil, err
	}
	if theme.mutedPen, err = walk.NewGeometricPen(walk.PenSolid|walk.PenCapRound, 2, mutedBrush); err != nil {
		return nil, err
	}
	if theme.textPen, err = walk.NewGeometricPen(walk.PenSolid|walk.PenCapRound, 4, textBrush); err != nil {
		return nil, err
	}

	if theme.brandFont, err = walk.NewFont("Verdana", 13, walk.FontBold); err != nil {
		return nil, err
	}
	if theme.titleFont, err = walk.NewFont("Verdana", 20, walk.FontBold); err != nil {
		return nil, err
	}
	if theme.headingFont, err = walk.NewFont("Verdana", 10, walk.FontBold); err != nil {
		return nil, err
	}
	if theme.bodyFont, err = walk.NewFont("Verdana", 10, 0); err != nil {
		return nil, err
	}
	if theme.smallFont, err = walk.NewFont("Verdana", 9, 0); err != nil {
		return nil, err
	}
	if theme.microFont, err = walk.NewFont("Verdana", 8, walk.FontBold); err != nil {
		return nil, err
	}
	if theme.iconFont, err = walk.NewFont("Segoe Fluent Icons", 14, 0); err != nil {
		return nil, err
	}
	if theme.powerFont, err = walk.NewFont("Segoe Fluent Icons", 35, 0); err != nil {
		return nil, err
	}
	return theme, nil
}

func (theme *dashboardTheme) Dispose() {
	for _, pen := range []*walk.GeometricPen{theme.borderPen, theme.accentPen, theme.successPen, theme.dangerPen, theme.mutedPen, theme.textPen} {
		if pen != nil {
			pen.Dispose()
		}
	}
	for _, font := range []*walk.Font{theme.brandFont, theme.titleFont, theme.headingFont, theme.bodyFont, theme.smallFont, theme.microFont, theme.iconFont, theme.powerFont} {
		if font != nil {
			font.Dispose()
		}
	}
	for _, brush := range []*walk.SolidColorBrush{theme.background, theme.sidebar, theme.surface, theme.surfaceHover, theme.surfaceSelected, theme.accentSoft, theme.accent, theme.success, theme.danger, theme.warning, theme.faint, theme.border} {
		if brush != nil {
			brush.Dispose()
		}
	}
}

func (dashboard *Dashboard) px(value int) int {
	return dashboard.IntFrom96DPI(value)
}

func (dashboard *Dashboard) addHit(id string, bounds walk.Rectangle, disabled bool) {
	dashboard.hits = append(dashboard.hits, dashboardHit{ID: id, Bounds: bounds, Disabled: disabled})
}

func (dashboard *Dashboard) drawText(canvas *walk.Canvas, text string, font *walk.Font, color walk.Color, bounds walk.Rectangle, format walk.DrawTextFormat) {
	if dashboard.queueGDI {
		dashboard.gdiCommands = append(dashboard.gdiCommands, dashboardGDICommand{
			text: text, font: font, color: color, bounds: bounds, format: format,
		})
		return
	}
	_ = canvas.DrawTextPixels(text, font, color, bounds, format|walk.TextNoPrefix)
}

func (dashboard *Dashboard) drawImage(canvas *walk.Canvas, image walk.Image, bounds walk.Rectangle) {
	if dashboard.queueGDI {
		dashboard.gdiCommands = append(dashboard.gdiCommands, dashboardGDICommand{image: image, bounds: bounds})
		return
	}
	_ = canvas.DrawImageStretchedPixels(image, bounds)
}

func (dashboard *Dashboard) flushGDICommands(canvas *walk.Canvas) {
	for _, command := range dashboard.gdiCommands {
		if command.image != nil {
			_ = canvas.DrawImageStretchedPixels(command.image, command.bounds)
			continue
		}
		_ = canvas.DrawTextPixels(command.text, command.font, command.color, command.bounds, command.format|walk.TextNoPrefix)
	}
	dashboard.gdiCommands = dashboard.gdiCommands[:0]
}

func (dashboard *Dashboard) disableSmooth() {
	if dashboard.smooth != nil {
		dashboard.smooth.Dispose()
		dashboard.smooth = nil
	}
}

func (dashboard *Dashboard) fillRounded(canvas *walk.Canvas, brush *walk.SolidColorBrush, bounds walk.Rectangle, radius int) {
	if dashboard.smooth != nil && dashboard.smooth.FillRoundedRectangle(brush.Color(), bounds, radius) {
		return
	}
	dashboard.disableSmooth()
	_ = canvas.FillRoundedRectanglePixels(brush, bounds, walk.Size{Width: radius, Height: radius})
}

func (dashboard *Dashboard) fillEllipse(canvas *walk.Canvas, brush *walk.SolidColorBrush, bounds walk.Rectangle) {
	if dashboard.smooth != nil && dashboard.smooth.FillEllipse(brush.Color(), bounds) {
		return
	}
	dashboard.disableSmooth()
	_ = canvas.FillEllipsePixels(brush, bounds)
}

func (dashboard *Dashboard) fillRectangleGradient(canvas *walk.Canvas, bounds walk.Rectangle, start, end walk.Color, direction int, fallback *walk.SolidColorBrush) {
	if dashboard.smooth != nil && dashboard.smooth.FillRectangleGradient(bounds, start, end, direction) {
		return
	}
	dashboard.disableSmooth()
	_ = canvas.FillRectanglePixels(fallback, bounds)
}

func (dashboard *Dashboard) fillRoundedGradient(canvas *walk.Canvas, bounds walk.Rectangle, radius int, start, end walk.Color, direction int, fallback *walk.SolidColorBrush) {
	if dashboard.smooth != nil && dashboard.smooth.FillRoundedGradient(bounds, radius, start, end, direction) {
		return
	}
	dashboard.disableSmooth()
	dashboard.fillRounded(canvas, fallback, bounds, radius)
}

func (dashboard *Dashboard) fillEllipseGradient(canvas *walk.Canvas, bounds walk.Rectangle, start, end walk.Color, direction int, fallback *walk.SolidColorBrush) {
	if dashboard.smooth != nil && dashboard.smooth.FillEllipseGradient(bounds, start, end, direction) {
		return
	}
	dashboard.disableSmooth()
	dashboard.fillEllipse(canvas, fallback, bounds)
}

func (dashboard *Dashboard) drawRounded(canvas *walk.Canvas, pen walk.Pen, bounds walk.Rectangle, radius int) {
	_ = canvas.DrawRoundedRectanglePixels(pen, bounds, walk.Size{Width: radius, Height: radius})
}

func (dashboard *Dashboard) interactiveBrush(id string, selected bool) *walk.SolidColorBrush {
	if selected {
		return dashboard.theme.surfaceSelected
	}
	if id != "" && (dashboard.hoverID == id || dashboard.pressedID == id) {
		return dashboard.theme.surfaceHover
	}
	return dashboard.theme.surface
}

func insetRectangle(bounds walk.Rectangle, amount int) walk.Rectangle {
	return walk.Rectangle{
		X:      bounds.X + amount,
		Y:      bounds.Y + amount,
		Width:  bounds.Width - amount*2,
		Height: bounds.Height - amount*2,
	}
}

func (dashboard *Dashboard) fillBorderedRounded(canvas *walk.Canvas, fill, border *walk.SolidColorBrush, bounds walk.Rectangle, radius, width int) {
	if dashboard.smooth != nil {
		if dashboard.smooth.FillRoundedRectangle(border.Color(), bounds, radius) {
			inner := insetRectangle(bounds, width)
			if dashboard.smooth.FillRoundedRectangle(fill.Color(), inner, radius-width) {
				return
			}
		}
		dashboard.disableSmooth()
	}
	dashboard.fillRounded(canvas, fill, bounds, radius)
}

func (dashboard *Dashboard) fillGradientBorderedRounded(canvas *walk.Canvas, fallback, border *walk.SolidColorBrush, bounds walk.Rectangle, radius, width int, start, end walk.Color) {
	if dashboard.smooth != nil && dashboard.smooth.FillRoundedRectangle(border.Color(), bounds, radius) {
		inner := insetRectangle(bounds, width)
		if dashboard.smooth.FillRoundedGradient(inner, radius-width, start, end, 2) {
			return
		}
	}
	dashboard.disableSmooth()
	dashboard.fillBorderedRounded(canvas, fallback, border, bounds, radius, width)
}

func (dashboard *Dashboard) fillEllipseRing(canvas *walk.Canvas, ring, center *walk.SolidColorBrush, bounds walk.Rectangle, width int) {
	dashboard.fillEllipse(canvas, ring, bounds)
	inner := insetRectangle(bounds, width)
	if inner.Width > 0 && inner.Height > 0 {
		dashboard.fillEllipse(canvas, center, inner)
	}
}

func (dashboard *Dashboard) fillEllipseGradientRing(canvas *walk.Canvas, ring, center *walk.SolidColorBrush, bounds walk.Rectangle, width int, start, end walk.Color) {
	dashboard.fillEllipseGradient(canvas, bounds, start, end, 2, ring)
	inner := insetRectangle(bounds, width)
	if inner.Width > 0 && inner.Height > 0 {
		dashboard.fillEllipse(canvas, center, inner)
	}
}

func (dashboard *Dashboard) drawCard(canvas *walk.Canvas, id string, bounds walk.Rectangle, selected, disabled bool) {
	brush := dashboard.interactiveBrush(id, selected)
	if disabled {
		brush = dashboard.theme.sidebar
	}
	border := dashboard.theme.border
	borderWidth := dashboard.px(1)
	if selected {
		border = dashboard.theme.accent
		borderWidth = dashboard.px(2)
	}
	if selected && !disabled {
		dashboard.fillGradientBorderedRounded(canvas, brush, border, bounds, dashboard.px(8), borderWidth, dashboard.theme.selectedTop, dashboard.theme.selectedEnd)
	} else if id != "" && !disabled && (dashboard.hoverID == id || dashboard.pressedID == id) {
		dashboard.fillGradientBorderedRounded(canvas, brush, border, bounds, dashboard.px(8), borderWidth, dashboard.theme.selectedEnd, dashboard.theme.surfaceHover.Color())
	} else {
		dashboard.fillBorderedRounded(canvas, brush, border, bounds, dashboard.px(8), borderWidth)
	}
	if dashboard.smooth == nil {
		if selected {
			dashboard.drawRounded(canvas, dashboard.theme.accentPen, bounds, dashboard.px(8))
		} else {
			dashboard.drawRounded(canvas, dashboard.theme.borderPen, bounds, dashboard.px(8))
		}
	}
	if id != "" {
		dashboard.addHit(id, bounds, disabled)
	}
}

func (dashboard *Dashboard) drawButton(canvas *walk.Canvas, id, label, icon string, bounds walk.Rectangle, primary, disabled bool) {
	brush := dashboard.interactiveBrush(id, primary)
	if primary {
		brush = dashboard.theme.accentSoft
		if dashboard.hoverID == id || dashboard.pressedID == id {
			brush = dashboard.theme.accent
		}
	}
	if disabled {
		brush = dashboard.theme.sidebar
	}
	if primary {
		dashboard.fillRoundedGradient(canvas, bounds, dashboard.px(7), dashboard.theme.selectedTop, dashboard.theme.accentSoft.Color(), 0, brush)
	} else {
		dashboard.fillBorderedRounded(canvas, brush, dashboard.theme.border, bounds, dashboard.px(7), dashboard.px(1))
	}
	if !primary && dashboard.smooth == nil {
		dashboard.drawRounded(canvas, dashboard.theme.borderPen, bounds, dashboard.px(7))
	}
	color := dashboard.theme.textColor
	if disabled {
		color = dashboard.theme.faintColor
	}
	textX := bounds.X + dashboard.px(14)
	if icon != "" {
		dashboard.drawText(canvas, icon, dashboard.theme.iconFont, color, walk.Rectangle{X: textX, Y: bounds.Y, Width: dashboard.px(22), Height: bounds.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		textX += dashboard.px(25)
	}
	dashboard.drawText(canvas, label, dashboard.theme.smallFont, color, walk.Rectangle{X: textX, Y: bounds.Y, Width: bounds.X + bounds.Width - textX - dashboard.px(10), Height: bounds.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	dashboard.addHit(id, bounds, disabled)
}

func (dashboard *Dashboard) ensureBacking(size walk.Size) error {
	dpi := dashboard.DPI()
	if dashboard.backing != nil && dashboard.backingCanvas != nil && dashboard.backingSize == size && dashboard.backingDPI == dpi {
		return nil
	}
	if dashboard.backingCanvas != nil {
		dashboard.backingCanvas.Dispose()
		dashboard.backingCanvas = nil
	}
	if dashboard.backing != nil {
		dashboard.backing.Dispose()
		dashboard.backing = nil
	}
	bitmap, err := walk.NewBitmapForDPI(size, dpi)
	if err != nil {
		return err
	}
	backingCanvas, err := walk.NewCanvasFromImage(bitmap)
	if err != nil {
		bitmap.Dispose()
		return err
	}
	dashboard.backing = bitmap
	dashboard.backingCanvas = backingCanvas
	dashboard.backingSize = size
	dashboard.backingDPI = dpi
	return nil
}

func (dashboard *Dashboard) paint(canvas *walk.Canvas, _ walk.Rectangle) error {
	if dashboard.theme == nil {
		return nil
	}
	bounds := dashboard.ClientBoundsPixels()
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return nil
	}
	if err := dashboard.ensureBacking(walk.Size{Width: bounds.Width, Height: bounds.Height}); err != nil {
		return err
	}
	if err := dashboard.paintFrame(dashboard.backingCanvas, bounds); err != nil {
		return err
	}
	if !win.BitBlt(
		canvas.HDC(), int32(bounds.X), int32(bounds.Y), int32(bounds.Width), int32(bounds.Height),
		dashboard.backingCanvas.HDC(), 0, 0, win.SRCCOPY,
	) {
		return fmt.Errorf("copy dashboard back buffer failed")
	}
	return nil
}

func (dashboard *Dashboard) paintFrame(canvas *walk.Canvas, bounds walk.Rectangle) error {
	dashboard.hits = dashboard.hits[:0]
	dashboard.gdiCommands = dashboard.gdiCommands[:0]
	dashboard.queueGDI = true
	dashboard.smooth = newSmoothCanvas(canvas.HDC())
	dashboard.fillRectangleGradient(canvas, bounds, dashboard.theme.backgroundTop, dashboard.theme.backgroundEnd, 2, dashboard.theme.background)

	titleBarHeight := dashboard.px(38)
	dashboard.drawTitleBar(canvas, walk.Rectangle{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: titleBarHeight})
	navWidth := dashboard.px(190)
	sidebarBounds := walk.Rectangle{X: bounds.X, Y: titleBarHeight, Width: navWidth, Height: bounds.Height - titleBarHeight}
	dashboard.fillRectangleGradient(canvas, sidebarBounds, dashboard.theme.sidebarTop, dashboard.theme.sidebarEnd, 1, dashboard.theme.sidebar)
	dashboard.drawSidebar(canvas, sidebarBounds)

	content := walk.Rectangle{
		X:      navWidth + dashboard.px(32),
		Y:      titleBarHeight + dashboard.px(22),
		Width:  bounds.Width - navWidth - dashboard.px(64),
		Height: bounds.Height - titleBarHeight - dashboard.px(44),
	}
	dashboard.drawHeader(canvas, content)
	switch dashboard.page {
	case pageRouting:
		dashboard.drawRoutingPage(canvas, content)
	case pageRules:
		dashboard.drawRulesPage(canvas, content)
	case pageProfiles:
		dashboard.drawProfilesPage(canvas, content)
	case pageDiagnostics:
		dashboard.drawDiagnosticsPage(canvas, content)
	default:
		dashboard.drawHomePage(canvas, content)
	}
	if dashboard.lastError != "" {
		dashboard.drawErrorBanner(canvas, bounds, navWidth)
	}
	dashboard.disableSmooth()
	dashboard.queueGDI = false
	dashboard.flushGDICommands(canvas)
	win.GdiFlush()
	return nil
}

func (dashboard *Dashboard) drawTitleBar(canvas *walk.Canvas, bounds walk.Rectangle) {
	dashboard.fillRectangleGradient(canvas, bounds, dashboard.theme.sidebarTop, dashboard.theme.backgroundTop, 0, dashboard.theme.sidebar)
	dashboard.addHit("window:drag", walk.Rectangle{X: bounds.X, Y: bounds.Y, Width: bounds.Width - dashboard.px(144), Height: bounds.Height}, false)

	buttonWidth := dashboard.px(48)
	controls := []struct {
		id    string
		glyph string
	}{
		{"window:minimize", "\ue921"},
		{"window:maximize", "\ue922"},
		{"window:close", "\ue8bb"},
	}
	if win.IsZoomed(dashboard.Form().Handle()) {
		controls[1].glyph = "\ue923"
	}
	startX := bounds.X + bounds.Width - buttonWidth*len(controls)
	for index, control := range controls {
		buttonBounds := walk.Rectangle{X: startX + index*buttonWidth, Y: bounds.Y, Width: buttonWidth, Height: bounds.Height}
		if dashboard.hoverID == control.id || dashboard.pressedID == control.id {
			brush := dashboard.theme.surfaceHover
			if control.id == "window:close" {
				brush = dashboard.theme.danger
			}
			dashboard.fillRectangleGradient(canvas, buttonBounds, brush.Color(), brush.Color(), 0, brush)
		}
		color := dashboard.theme.mutedColor
		if dashboard.hoverID == control.id || dashboard.pressedID == control.id {
			color = dashboard.theme.textColor
		}
		dashboard.drawText(canvas, control.glyph, dashboard.theme.iconFont, color, buttonBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		dashboard.addHit(control.id, buttonBounds, false)
	}
	lineBounds := walk.Rectangle{X: bounds.X, Y: bounds.Y + bounds.Height - dashboard.px(1), Width: bounds.Width, Height: dashboard.px(1)}
	dashboard.fillRectangleGradient(canvas, lineBounds, dashboard.theme.border.Color(), dashboard.theme.border.Color(), 0, dashboard.theme.border)
}

type navItem struct {
	page  dashboardPage
	id    string
	label string
	icon  string
}

func (dashboard *Dashboard) drawSidebar(canvas *walk.Canvas, bounds walk.Rectangle) {
	logoSize := dashboard.px(38)
	logoBounds := walk.Rectangle{X: dashboard.px(20), Y: bounds.Y + dashboard.px(20), Width: logoSize, Height: logoSize}
	if icon, err := loadLogoIcon(48); err == nil {
		dashboard.drawImage(canvas, icon, logoBounds)
	} else {
		dashboard.fillRounded(canvas, dashboard.theme.warning, logoBounds, dashboard.px(10))
		dashboard.drawText(canvas, "P", dashboard.theme.brandFont, walk.RGB(8, 20, 35), logoBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	}
	dashboard.drawText(canvas, "PINUS", dashboard.theme.brandFont, dashboard.theme.textColor, walk.Rectangle{X: dashboard.px(68), Y: bounds.Y + dashboard.px(26), Width: dashboard.px(100), Height: dashboard.px(26)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)

	items := []navItem{
		{pageHome, "nav:home", "Главная", "\ue80f"},
		{pageRouting, "nav:routing", "Маршрутизация", "\ue8ab"},
		{pageRules, "nav:rules", "Правила", "\ue8fd"},
		{pageProfiles, "nav:profiles", "Профили", "\ue8d4"},
	}
	itemY := bounds.Y + dashboard.px(104)
	for _, item := range items {
		rect := walk.Rectangle{X: dashboard.px(10), Y: itemY, Width: bounds.Width - dashboard.px(20), Height: dashboard.px(46)}
		selected := dashboard.page == item.page
		if selected || dashboard.hoverID == item.id {
			fallback := dashboard.interactiveBrush(item.id, selected)
			start := dashboard.theme.selectedEnd
			end := dashboard.theme.surfaceHover.Color()
			if selected {
				start = dashboard.theme.selectedTop
				end = dashboard.theme.selectedEnd
			}
			dashboard.fillRoundedGradient(canvas, rect, dashboard.px(7), start, end, 0, fallback)
		}
		if selected {
			dashboard.fillRounded(canvas, dashboard.theme.accent, walk.Rectangle{X: rect.X, Y: rect.Y + dashboard.px(8), Width: dashboard.px(4), Height: rect.Height - dashboard.px(16)}, dashboard.px(2))
		}
		color := dashboard.theme.textColor
		if !selected {
			color = dashboard.theme.mutedColor
		}
		dashboard.drawText(canvas, item.icon, dashboard.theme.iconFont, color, walk.Rectangle{X: rect.X + dashboard.px(16), Y: rect.Y, Width: dashboard.px(24), Height: rect.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		dashboard.drawText(canvas, item.label, dashboard.theme.bodyFont, color, walk.Rectangle{X: rect.X + dashboard.px(48), Y: rect.Y, Width: rect.Width - dashboard.px(58), Height: rect.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		dashboard.addHit(item.id, rect, false)
		itemY += dashboard.px(52)
	}

	diagnosticBounds := walk.Rectangle{X: dashboard.px(10), Y: bounds.Y + bounds.Height - dashboard.px(62), Width: bounds.Width - dashboard.px(20), Height: dashboard.px(44)}
	selected := dashboard.page == pageDiagnostics
	if selected || dashboard.hoverID == "nav:diagnostics" {
		dashboard.fillRounded(canvas, dashboard.interactiveBrush("nav:diagnostics", selected), diagnosticBounds, dashboard.px(7))
	}
	dashboard.drawText(canvas, "\ue713", dashboard.theme.iconFont, dashboard.theme.mutedColor, walk.Rectangle{X: diagnosticBounds.X + dashboard.px(16), Y: diagnosticBounds.Y, Width: dashboard.px(24), Height: diagnosticBounds.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	dashboard.drawText(canvas, "Диагностика", dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: diagnosticBounds.X + dashboard.px(48), Y: diagnosticBounds.Y, Width: diagnosticBounds.Width - dashboard.px(58), Height: diagnosticBounds.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	dashboard.addHit("nav:diagnostics", diagnosticBounds, false)
}

func pageCopy(page dashboardPage) (string, string) {
	switch page {
	case pageRouting:
		return "Маршрутизация", "Выбери, какой трафик пойдёт через VPN"
	case pageRules:
		return "Свои правила", "Приложения, домены и IP с явным направлением"
	case pageProfiles:
		return "VPN-профили", "Выбор сервера и управление импортированными конфигами"
	case pageDiagnostics:
		return "Диагностика", "Состояние компонентов без приватных ключей"
	default:
		return "Подключение", "VPN и умная маршрутизация в одном окне"
	}
}

func (dashboard *Dashboard) drawHeader(canvas *walk.Canvas, content walk.Rectangle) {
	title, subtitle := pageCopy(dashboard.page)
	dashboard.drawText(canvas, title, dashboard.theme.titleFont, dashboard.theme.textColor, walk.Rectangle{X: content.X, Y: content.Y, Width: content.Width, Height: dashboard.px(34)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	dashboard.drawText(canvas, subtitle, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: content.X, Y: content.Y + dashboard.px(34), Width: content.Width, Height: dashboard.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
}

func stateCopy(state manager.TunnelState, busy bool) (string, string, walk.Color) {
	if busy || state == manager.TunnelStarting || state == manager.TunnelStopping {
		if state == manager.TunnelStopping {
			return "Отключаем VPN", "Закрываем маршруты и сетевой интерфейс", walk.RGB(240, 182, 76)
		}
		return "Подключаем VPN", "Проверяем профиль и поднимаем маршруты", walk.RGB(54, 169, 255)
	}
	if state == manager.TunnelStarted {
		return "VPN подключён", "Трафик идёт по выбранным правилам", walk.RGB(46, 210, 139)
	}
	return "VPN отключён", "Трафик использует обычное подключение", walk.RGB(158, 181, 205)
}

func modeCopy(mode smart.Mode) (string, string) {
	switch smart.NormalizeMode(string(mode)) {
	case smart.ModeSelected:
		return "Только выбранное", "VPN используют выбранные сервисы, приложения и правила"
	default:
		return "Весь интернет", "Весь трафик через VPN; свои правила могут создать исключения"
	}
}

func serviceSectionCopy(mode smart.Mode) (string, string) {
	if serviceSelectionEnabled(mode) {
		return "Сервисы через VPN", "Остальной трафик будет идти напрямую"
	}
	return "Сервисы", "Не применяются в режиме «Весь интернет»"
}

func routingSummaryCopy(settings smart.RoutingSettings) string {
	if !serviceSelectionEnabled(settings.Mode) {
		if smart.HasEnabledCustomRules(settings) {
			return fmt.Sprintf("Весь трафик через VPN · активных правил: %d", enabledRuleCount(settings.CustomRules))
		}
		return "Нативный AWG · без дополнительной маршрутизации"
	}
	return fmt.Sprintf("Сервисов: %d · Правил: %d", len(settings.SelectedApps), len(settings.CustomRules))
}

func formatSessionDuration(start time.Time) string {
	if start.IsZero() {
		return "00:00:00"
	}
	duration := time.Since(start)
	if duration < 0 {
		duration = 0
	}
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func mixColor(from, to walk.Color, amount float64) walk.Color {
	if amount < 0 {
		amount = 0
	} else if amount > 1 {
		amount = 1
	}
	channel := func(value walk.Color, shift uint) float64 {
		return float64((uint32(value) >> shift) & 0xff)
	}
	mix := func(left, right float64) uint8 {
		return uint8(left + (right-left)*amount + 0.5)
	}
	return walk.RGB(
		mix(channel(from, 0), channel(to, 0)),
		mix(channel(from, 8), channel(to, 8)),
		mix(channel(from, 16), channel(to, 16)),
	)
}

func (dashboard *Dashboard) drawHomePage(canvas *walk.Canvas, content walk.Rectangle) {
	startY := content.Y + dashboard.px(70)
	profileBounds := walk.Rectangle{X: content.X, Y: startY, Width: content.Width, Height: dashboard.px(72)}
	dashboard.drawCard(canvas, "home:profile", profileBounds, false, dashboard.operationBusy())
	profileName := "Добавь VPN-профиль"
	endpoint := "Без профиля подключение недоступно"
	if profile := dashboard.selectedProfile(); profile != nil {
		profileName = profile.Tunnel.Name
		endpoint = profile.Endpoint
	}
	dashboard.drawText(canvas, "\ue8d4", dashboard.theme.iconFont, dashboard.theme.accentColor, walk.Rectangle{X: profileBounds.X + dashboard.px(18), Y: profileBounds.Y, Width: dashboard.px(28), Height: profileBounds.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	dashboard.drawText(canvas, profileName, dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: profileBounds.X + dashboard.px(58), Y: profileBounds.Y + dashboard.px(12), Width: profileBounds.Width - dashboard.px(125), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	dashboard.drawText(canvas, endpoint, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: profileBounds.X + dashboard.px(58), Y: profileBounds.Y + dashboard.px(37), Width: profileBounds.Width - dashboard.px(125), Height: dashboard.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	dashboard.drawText(canvas, "\ue76c", dashboard.theme.iconFont, dashboard.theme.mutedColor, walk.Rectangle{X: profileBounds.X + profileBounds.Width - dashboard.px(48), Y: profileBounds.Y, Width: dashboard.px(28), Height: profileBounds.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)

	centerX := content.X + content.Width/2
	centerOffset := dashboard.px(206)
	if content.Height < dashboard.px(600) {
		centerOffset = dashboard.px(184)
	}
	centerY := startY + centerOffset
	radius := dashboard.px(78)
	busy := dashboard.operationBusy()
	connected := dashboard.globalState == manager.TunnelStarted
	disabled := dashboard.selectedProfile() == nil || busy
	powerBounds := walk.Rectangle{X: centerX - radius - dashboard.px(18), Y: centerY - radius - dashboard.px(18), Width: (radius + dashboard.px(18)) * 2, Height: (radius + dashboard.px(18)) * 2}
	dashboard.addHit("power", powerBounds, disabled)

	if busy {
		for i := 0; i < 12; i++ {
			angle := dashboard.animation*1.8 + float64(i)*math.Pi/6
			dotRadius := dashboard.px(2)
			x := centerX + int(math.Cos(angle)*float64(radius+dashboard.px(10)))
			y := centerY + int(math.Sin(angle)*float64(radius+dashboard.px(10)))
			brush := dashboard.theme.faint
			if i < 4 {
				brush = dashboard.theme.accent
			}
			dashboard.fillEllipse(canvas, brush, walk.Rectangle{X: x - dotRadius, Y: y - dotRadius, Width: dotRadius * 2, Height: dotRadius * 2})
		}
	} else {
		ringBounds := walk.Rectangle{X: centerX - radius, Y: centerY - radius, Width: radius * 2, Height: radius * 2}
		ringBrush := dashboard.theme.accent
		if connected {
			ringBrush = dashboard.theme.success
		}
		if disabled {
			ringBrush = dashboard.theme.faint
		}
		ringStart := dashboard.theme.accentColor
		ringEnd := walk.RGB(66, 211, 224)
		if connected {
			phase := (math.Sin(dashboard.animation*2.1) + 1) / 2
			ringStart = mixColor(dashboard.theme.successColor, walk.RGB(73, 232, 183), phase*0.55)
			ringEnd = mixColor(dashboard.theme.accentColor, walk.RGB(66, 211, 224), phase*0.45)
		}
		if disabled {
			ringStart = dashboard.theme.faintColor
			ringEnd = dashboard.theme.faint.Color()
		}
		dashboard.fillEllipseGradientRing(canvas, ringBrush, dashboard.theme.background, ringBounds, dashboard.px(3), ringStart, ringEnd)
	}
	innerRadius := dashboard.px(58)
	innerBounds := walk.Rectangle{X: centerX - innerRadius, Y: centerY - innerRadius, Width: innerRadius * 2, Height: innerRadius * 2}
	innerBrush := dashboard.theme.surface
	if dashboard.hoverID == "power" && !disabled {
		innerBrush = dashboard.theme.surfaceHover
	}
	powerStart := dashboard.theme.powerTop
	powerEnd := dashboard.theme.powerEnd
	if dashboard.hoverID == "power" && !disabled {
		powerStart = dashboard.theme.selectedTop
		powerEnd = dashboard.theme.selectedEnd
	}
	if connected {
		powerStart = walk.RGB(18, 101, 88)
		powerEnd = walk.RGB(13, 54, 70)
	}
	dashboard.fillEllipseGradient(canvas, innerBounds, powerStart, powerEnd, 2, innerBrush)
	powerColor := dashboard.theme.textColor
	if disabled {
		powerColor = dashboard.theme.faintColor
	} else if connected {
		powerColor = dashboard.theme.successColor
	}
	dashboard.drawText(canvas, "\ue7e8", dashboard.theme.powerFont, powerColor, innerBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)

	status, statusDetail, statusColor := stateCopy(dashboard.globalState, busy)
	if connected && smart.NormalizeMode(string(dashboard.settings.Mode)) == smart.ModeAll {
		statusDetail = "Весь интернет идёт через VPN"
		if smart.HasEnabledCustomRules(dashboard.settings) {
			statusDetail = "Весь интернет через VPN, свои правила применяются раньше"
		}
	}
	dashboard.drawText(canvas, status, dashboard.theme.headingFont, statusColor, walk.Rectangle{X: content.X, Y: centerY + dashboard.px(96), Width: content.Width, Height: dashboard.px(26)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	dashboard.drawText(canvas, statusDetail, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: content.X, Y: centerY + dashboard.px(122), Width: content.Width, Height: dashboard.px(22)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

	cardY := content.Y + content.Height - dashboard.px(120)
	gap := dashboard.px(14)
	cardWidth := (content.Width - gap) / 2
	routingBounds := walk.Rectangle{X: content.X, Y: cardY, Width: cardWidth, Height: dashboard.px(98)}
	dashboard.drawCard(canvas, "home:routing", routingBounds, false, dashboard.selectedProfile() == nil || busy)
	modeTitle, _ := modeCopy(dashboard.settings.Mode)
	dashboard.drawText(canvas, "МАРШРУТИЗАЦИЯ", dashboard.theme.microFont, dashboard.theme.accentColor, walk.Rectangle{X: routingBounds.X + dashboard.px(18), Y: routingBounds.Y + dashboard.px(14), Width: routingBounds.Width - dashboard.px(36), Height: dashboard.px(18)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	dashboard.drawText(canvas, modeTitle, dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: routingBounds.X + dashboard.px(18), Y: routingBounds.Y + dashboard.px(35), Width: routingBounds.Width - dashboard.px(60), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	dashboard.drawText(canvas, routingSummaryCopy(dashboard.settings), dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: routingBounds.X + dashboard.px(18), Y: routingBounds.Y + dashboard.px(62), Width: routingBounds.Width - dashboard.px(36), Height: dashboard.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

	sessionBounds := walk.Rectangle{X: routingBounds.X + routingBounds.Width + gap, Y: cardY, Width: cardWidth, Height: dashboard.px(98)}
	dashboard.drawCard(canvas, "", sessionBounds, false, false)
	dashboard.drawText(canvas, "СЕССИЯ", dashboard.theme.microFont, dashboard.theme.successColor, walk.Rectangle{X: sessionBounds.X + dashboard.px(18), Y: sessionBounds.Y + dashboard.px(14), Width: sessionBounds.Width - dashboard.px(36), Height: dashboard.px(18)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	dashboard.drawText(canvas, formatSessionDuration(dashboard.connectedAt), dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: sessionBounds.X + dashboard.px(18), Y: sessionBounds.Y + dashboard.px(35), Width: sessionBounds.Width - dashboard.px(36), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	sessionText := "Нет активного подключения"
	if connected {
		sessionText = "Соединение активно"
	}
	dashboard.drawText(canvas, sessionText, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: sessionBounds.X + dashboard.px(18), Y: sessionBounds.Y + dashboard.px(62), Width: sessionBounds.Width - dashboard.px(36), Height: dashboard.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
}

func (dashboard *Dashboard) drawRoutingPage(canvas *walk.Canvas, content walk.Rectangle) {
	startY := content.Y + dashboard.px(76)
	gap := dashboard.px(12)
	modeTitles := []string{"Весь интернет", "Только выбранное"}
	modeDetails := []string{"Весь трафик через VPN", "Остальное напрямую"}
	cardWidth := (content.Width - gap) / len(modeTitles)
	busy := dashboard.operationBusy()
	for i := range modeTitles {
		rect := walk.Rectangle{X: content.X + i*(cardWidth+gap), Y: startY, Width: cardWidth, Height: dashboard.px(106)}
		selected := smart.IndexOfMode(dashboard.settings.Mode) == i
		id := fmt.Sprintf("mode:%d", i)
		disabled := busy || dashboard.selectedProfile() == nil
		dashboard.drawCard(canvas, id, rect, selected, disabled)
		dotBounds := walk.Rectangle{X: rect.X + dashboard.px(17), Y: rect.Y + dashboard.px(17), Width: dashboard.px(16), Height: dashboard.px(16)}
		cardBrush := dashboard.interactiveBrush(id, selected)
		if disabled {
			cardBrush = dashboard.theme.sidebar
		}
		dashboard.fillEllipseRing(canvas, dashboard.theme.faint, cardBrush, dotBounds, dashboard.px(2))
		if selected {
			inner := walk.Rectangle{X: dotBounds.X + dashboard.px(4), Y: dotBounds.Y + dashboard.px(4), Width: dotBounds.Width - dashboard.px(8), Height: dotBounds.Height - dashboard.px(8)}
			dashboard.fillEllipse(canvas, dashboard.theme.accent, inner)
		}
		dashboard.drawText(canvas, modeTitles[i], dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: rect.X + dashboard.px(17), Y: rect.Y + dashboard.px(43), Width: rect.Width - dashboard.px(34), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		dashboard.drawText(canvas, modeDetails[i], dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: rect.X + dashboard.px(17), Y: rect.Y + dashboard.px(68), Width: rect.Width - dashboard.px(34), Height: dashboard.px(25)}, walk.TextLeft|walk.TextVCenter|walk.TextWordbreak|walk.TextEndEllipsis)
	}

	sectionY := startY + dashboard.px(132)
	sectionTitle, sectionDetail := serviceSectionCopy(dashboard.settings.Mode)
	dashboard.drawText(canvas, sectionTitle, dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: content.X, Y: sectionY, Width: content.Width / 2, Height: dashboard.px(26)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	dashboard.drawText(canvas, sectionDetail, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: content.X + content.Width/2, Y: sectionY, Width: content.Width / 2, Height: dashboard.px(26)}, walk.TextRight|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)

	services := smart.ServiceCatalog()
	gridY := sectionY + dashboard.px(38)
	serviceColumns := 2
	if len(services) > 4 {
		serviceColumns = 3
	}
	serviceWidth := (content.Width - gap*(serviceColumns-1)) / serviceColumns
	for i, service := range services {
		column := i % serviceColumns
		row := i / serviceColumns
		rect := walk.Rectangle{X: content.X + column*(serviceWidth+gap), Y: gridY + row*dashboard.px(90), Width: serviceWidth, Height: dashboard.px(76)}
		id := "service:" + service.ID
		selectionEnabled := serviceSelectionEnabled(dashboard.settings.Mode)
		selected := selectionEnabled && containsString(dashboard.settings.SelectedApps, service.ID)
		disabled := busy || dashboard.selectedProfile() == nil || !selectionEnabled
		dashboard.drawCard(canvas, id, rect, selected, disabled)
		dashboard.drawText(canvas, service.Name, dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: rect.X + dashboard.px(17), Y: rect.Y + dashboard.px(12), Width: rect.Width - dashboard.px(90), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		dashboard.drawText(canvas, service.Description, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: rect.X + dashboard.px(17), Y: rect.Y + dashboard.px(39), Width: rect.Width - dashboard.px(90), Height: dashboard.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		dashboard.drawToggle(canvas, id, walk.Rectangle{X: rect.X + rect.Width - dashboard.px(60), Y: rect.Y + dashboard.px(24), Width: dashboard.px(42), Height: dashboard.px(24)}, selected, disabled)
	}

	serviceRows := (len(services) + serviceColumns - 1) / serviceColumns
	rulesY := gridY + serviceRows*dashboard.px(90) + dashboard.px(10)
	rulesBounds := walk.Rectangle{X: content.X, Y: rulesY, Width: content.Width, Height: dashboard.px(72)}
	dashboard.drawCard(canvas, "routing:rules", rulesBounds, false, dashboard.selectedProfile() == nil || busy)
	dashboard.drawText(canvas, "Свои правила", dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: rulesBounds.X + dashboard.px(17), Y: rulesBounds.Y + dashboard.px(11), Width: rulesBounds.Width - dashboard.px(150), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	rulesDetail := fmt.Sprintf("Активно %d из %d", enabledRuleCount(dashboard.settings.CustomRules), len(dashboard.settings.CustomRules))
	if smart.NormalizeMode(string(dashboard.settings.Mode)) == smart.ModeAll {
		rulesDetail += " · исключения применяются раньше общего VPN-маршрута"
	}
	dashboard.drawText(canvas, rulesDetail, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: rulesBounds.X + dashboard.px(17), Y: rulesBounds.Y + dashboard.px(38), Width: rulesBounds.Width - dashboard.px(150), Height: dashboard.px(21)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	dashboard.drawText(canvas, "\ue76c", dashboard.theme.iconFont, dashboard.theme.mutedColor, walk.Rectangle{X: rulesBounds.X + rulesBounds.Width - dashboard.px(48), Y: rulesBounds.Y, Width: dashboard.px(28), Height: rulesBounds.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
}

func enabledRuleCount(rules []smart.CustomRule) int {
	count := 0
	for _, rule := range rules {
		if rule.Enabled {
			count++
		}
	}
	return count
}

func (dashboard *Dashboard) drawToggle(canvas *walk.Canvas, id string, bounds walk.Rectangle, checked, disabled bool) {
	brush := dashboard.theme.faint
	if checked {
		brush = dashboard.theme.accent
	}
	if disabled {
		brush = dashboard.theme.sidebar
	}
	if checked && !disabled {
		dashboard.fillRoundedGradient(canvas, bounds, bounds.Height, dashboard.theme.accentColor, walk.RGB(74, 215, 231), 0, brush)
	} else {
		dashboard.fillRounded(canvas, brush, bounds, bounds.Height)
	}
	knob := bounds.Height - dashboard.px(6)
	knobX := bounds.X + dashboard.px(3)
	if checked {
		knobX = bounds.X + bounds.Width - knob - dashboard.px(3)
	}
	dashboard.fillEllipse(canvas, dashboard.theme.surface, walk.Rectangle{X: knobX, Y: bounds.Y + dashboard.px(3), Width: knob, Height: knob})
	dashboard.addHit(id, bounds, disabled)
}

func (dashboard *Dashboard) drawScrollIndicator(canvas *walk.Canvas, bounds walk.Rectangle, total, visible, offset int) {
	if total <= visible || visible <= 0 || bounds.Height <= 0 {
		return
	}
	trackWidth := dashboard.px(4)
	track := walk.Rectangle{X: bounds.X + bounds.Width - trackWidth, Y: bounds.Y, Width: trackWidth, Height: bounds.Height}
	dashboard.fillRounded(canvas, dashboard.theme.faint, track, trackWidth)
	thumbHeight := bounds.Height * visible / total
	minimum := dashboard.px(28)
	if thumbHeight < minimum {
		thumbHeight = minimum
	}
	if thumbHeight > bounds.Height {
		thumbHeight = bounds.Height
	}
	maxOffset := total - visible
	thumbY := bounds.Y
	if maxOffset > 0 {
		thumbY += (bounds.Height - thumbHeight) * offset / maxOffset
	}
	thumb := walk.Rectangle{X: track.X, Y: thumbY, Width: track.Width, Height: thumbHeight}
	dashboard.fillRoundedGradient(canvas, thumb, trackWidth, dashboard.theme.accentColor, walk.RGB(70, 205, 225), 1, dashboard.theme.accent)
}

func ruleKindCopy(kind smart.RuleKind) (string, string) {
	switch kind {
	case smart.RuleApplication:
		return "Приложение", "\ue8a5"
	case smart.RuleDomain:
		return "Домен", "\ue774"
	default:
		return "IP / CIDR", "\ue128"
	}
}

func (dashboard *Dashboard) drawRulesPage(canvas *walk.Canvas, content walk.Rectangle) {
	startY := content.Y + dashboard.px(74)
	buttonWidth := dashboard.px(154)
	dashboard.drawButton(canvas, "rules:add", "Добавить правило", "\ue710", walk.Rectangle{X: content.X + content.Width - buttonWidth, Y: startY, Width: buttonWidth, Height: dashboard.px(38)}, true, dashboard.selectedProfile() == nil || dashboard.operationBusy())
	dashboard.drawText(canvas, fmt.Sprintf("Правил: %d", len(dashboard.settings.CustomRules)), dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: content.X, Y: startY, Width: content.Width - buttonWidth - dashboard.px(16), Height: dashboard.px(38)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)

	listY := startY + dashboard.px(54)
	if len(dashboard.settings.CustomRules) == 0 {
		empty := walk.Rectangle{X: content.X, Y: listY, Width: content.Width, Height: dashboard.px(190)}
		dashboard.drawCard(canvas, "", empty, false, false)
		dashboard.drawText(canvas, "\ue8fd", dashboard.theme.iconFont, dashboard.theme.accentColor, walk.Rectangle{X: empty.X, Y: empty.Y + dashboard.px(28), Width: empty.Width, Height: dashboard.px(40)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		dashboard.drawText(canvas, "Своих правил пока нет", dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: empty.X + dashboard.px(20), Y: empty.Y + dashboard.px(76), Width: empty.Width - dashboard.px(40), Height: dashboard.px(28)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		dashboard.drawText(canvas, "Добавь EXE, домен или IP и выбери: VPN либо напрямую", dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: empty.X + dashboard.px(40), Y: empty.Y + dashboard.px(108), Width: empty.Width - dashboard.px(80), Height: dashboard.px(42)}, walk.TextCenter|walk.TextVCenter|walk.TextWordbreak)
		return
	}

	rowHeight := dashboard.px(66)
	gap := dashboard.px(9)
	visibleHeight := content.Y + content.Height - listY - dashboard.px(8)
	visibleCount := visibleHeight / (rowHeight + gap)
	if visibleCount < 1 {
		visibleCount = 1
	}
	maxScroll := len(dashboard.settings.CustomRules) - visibleCount
	if maxScroll < 0 {
		maxScroll = 0
	}
	if dashboard.ruleScroll > maxScroll {
		dashboard.ruleScroll = maxScroll
	}
	end := dashboard.ruleScroll + visibleCount
	if end > len(dashboard.settings.CustomRules) {
		end = len(dashboard.settings.CustomRules)
	}
	for index := dashboard.ruleScroll; index < end; index++ {
		rule := dashboard.settings.CustomRules[index]
		y := listY + (index-dashboard.ruleScroll)*(rowHeight+gap)
		rect := walk.Rectangle{X: content.X, Y: y, Width: content.Width, Height: rowHeight}
		dashboard.drawCard(canvas, "", rect, false, false)
		kindLabel, kindIcon := ruleKindCopy(rule.Kind)
		iconBounds := walk.Rectangle{X: rect.X + dashboard.px(14), Y: rect.Y + dashboard.px(13), Width: dashboard.px(40), Height: dashboard.px(40)}
		dashboard.fillRounded(canvas, dashboard.theme.accentSoft, iconBounds, dashboard.px(8))
		dashboard.drawText(canvas, kindIcon, dashboard.theme.iconFont, dashboard.theme.accentColor, iconBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		infoWidth := rect.Width - dashboard.px(315)
		dashboard.drawText(canvas, rule.Name, dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: rect.X + dashboard.px(66), Y: rect.Y + dashboard.px(9), Width: infoWidth, Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		dashboard.drawText(canvas, kindLabel+" · "+rule.Value, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: rect.X + dashboard.px(66), Y: rect.Y + dashboard.px(34), Width: infoWidth, Height: dashboard.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextPathEllipsis)

		targetText := "VPN"
		targetColor := dashboard.theme.accentColor
		if rule.Target == smart.TargetDirect {
			targetText = "НАПРЯМУЮ"
			targetColor = dashboard.theme.warningColor
		}
		targetBounds := walk.Rectangle{X: rect.X + rect.Width - dashboard.px(252), Y: rect.Y + dashboard.px(20), Width: dashboard.px(88), Height: dashboard.px(26)}
		dashboard.fillRounded(canvas, dashboard.theme.sidebar, targetBounds, dashboard.px(13))
		dashboard.drawText(canvas, targetText, dashboard.theme.microFont, targetColor, targetBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		toggleID := fmt.Sprintf("rule:toggle:%d", index)
		dashboard.drawToggle(canvas, toggleID, walk.Rectangle{X: rect.X + rect.Width - dashboard.px(151), Y: rect.Y + dashboard.px(21), Width: dashboard.px(42), Height: dashboard.px(24)}, rule.Enabled, dashboard.operationBusy())
		editID := fmt.Sprintf("rule:edit:%d", index)
		deleteID := fmt.Sprintf("rule:delete:%d", index)
		dashboard.drawIconButton(canvas, editID, "\ue70f", walk.Rectangle{X: rect.X + rect.Width - dashboard.px(98), Y: rect.Y + dashboard.px(15), Width: dashboard.px(38), Height: dashboard.px(36)}, false)
		dashboard.drawIconButton(canvas, deleteID, "\ue74d", walk.Rectangle{X: rect.X + rect.Width - dashboard.px(50), Y: rect.Y + dashboard.px(15), Width: dashboard.px(38), Height: dashboard.px(36)}, true)
	}
	dashboard.drawScrollIndicator(canvas, walk.Rectangle{X: content.X, Y: listY, Width: content.Width, Height: visibleHeight}, len(dashboard.settings.CustomRules), visibleCount, dashboard.ruleScroll)
}

func (dashboard *Dashboard) drawIconButton(canvas *walk.Canvas, id, icon string, bounds walk.Rectangle, danger bool) {
	brush := dashboard.theme.sidebar
	if dashboard.hoverID == id || dashboard.pressedID == id {
		brush = dashboard.theme.surfaceHover
	}
	dashboard.fillRounded(canvas, brush, bounds, dashboard.px(6))
	color := dashboard.theme.mutedColor
	if danger {
		color = dashboard.theme.dangerColor
	}
	dashboard.drawText(canvas, icon, dashboard.theme.iconFont, color, bounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	dashboard.addHit(id, bounds, dashboard.operationBusy())
}

func profileStateCopy(state manager.TunnelState) (string, walk.Color) {
	switch state {
	case manager.TunnelStarted:
		return "ПОДКЛЮЧЁН", walk.RGB(46, 210, 139)
	case manager.TunnelStarting, manager.TunnelStopping:
		return "ПЕРЕКЛЮЧЕНИЕ", walk.RGB(240, 182, 76)
	default:
		return "ГОТОВ", walk.RGB(158, 181, 205)
	}
}

func (dashboard *Dashboard) drawProfilesPage(canvas *walk.Canvas, content walk.Rectangle) {
	startY := content.Y + dashboard.px(74)
	buttonWidth := dashboard.px(174)
	dashboard.drawButton(canvas, "profiles:import", "Импорт .conf / .zip", "\ue8e5", walk.Rectangle{X: content.X + content.Width - buttonWidth, Y: startY, Width: buttonWidth, Height: dashboard.px(38)}, true, dashboard.operationBusy())
	dashboard.drawText(canvas, fmt.Sprintf("Профилей: %d", len(dashboard.profiles)), dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: content.X, Y: startY, Width: content.Width - buttonWidth - dashboard.px(16), Height: dashboard.px(38)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)

	listY := startY + dashboard.px(54)
	if len(dashboard.profiles) == 0 {
		empty := walk.Rectangle{X: content.X, Y: listY, Width: content.Width, Height: dashboard.px(224)}
		dashboard.drawCard(canvas, "", empty, false, false)
		dashboard.drawText(canvas, "\ue8d4", dashboard.theme.iconFont, dashboard.theme.accentColor, walk.Rectangle{X: empty.X, Y: empty.Y + dashboard.px(20), Width: empty.Width, Height: dashboard.px(42)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		dashboard.drawText(canvas, "Добавь первый VPN-профиль", dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: empty.X + dashboard.px(20), Y: empty.Y + dashboard.px(68), Width: empty.Width - dashboard.px(40), Height: dashboard.px(28)}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		dashboard.drawText(canvas, "Импортируй свой .conf / .zip или получи готовый профиль у Pinus VPN", dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: empty.X + dashboard.px(40), Y: empty.Y + dashboard.px(100), Width: empty.Width - dashboard.px(80), Height: dashboard.px(38)}, walk.TextCenter|walk.TextVCenter|walk.TextWordbreak)

		buttonGap := dashboard.px(12)
		importWidth := dashboard.px(174)
		buyWidth := dashboard.px(166)
		buttonsWidth := importWidth + buttonGap + buyWidth
		buttonsX := empty.X + (empty.Width-buttonsWidth)/2
		buttonsY := empty.Y + dashboard.px(156)
		dashboard.drawButton(canvas, "profiles:import", "Импортировать", "\ue8e5", walk.Rectangle{X: buttonsX, Y: buttonsY, Width: importWidth, Height: dashboard.px(42)}, false, dashboard.operationBusy())
		dashboard.drawButton(canvas, "profiles:buy", "Купить профиль", "\ue8d4", walk.Rectangle{X: buttonsX + importWidth + buttonGap, Y: buttonsY, Width: buyWidth, Height: dashboard.px(42)}, true, false)
		return
	}

	rowHeight := dashboard.px(72)
	gap := dashboard.px(10)
	visibleHeight := content.Y + content.Height - listY - dashboard.px(8)
	visibleCount := visibleHeight / (rowHeight + gap)
	if visibleCount < 1 {
		visibleCount = 1
	}
	maxScroll := len(dashboard.profiles) - visibleCount
	if maxScroll < 0 {
		maxScroll = 0
	}
	if dashboard.profileScroll > maxScroll {
		dashboard.profileScroll = maxScroll
	}
	end := dashboard.profileScroll + visibleCount
	if end > len(dashboard.profiles) {
		end = len(dashboard.profiles)
	}
	for index := dashboard.profileScroll; index < end; index++ {
		profile := dashboard.profiles[index]
		y := listY + (index-dashboard.profileScroll)*(rowHeight+gap)
		rect := walk.Rectangle{X: content.X, Y: y, Width: content.Width, Height: rowHeight}
		id := fmt.Sprintf("profile:select:%d", index)
		selected := index == dashboard.selected
		dashboard.drawCard(canvas, id, rect, selected, dashboard.operationBusy())
		stateText, stateColor := profileStateCopy(profile.State)
		dotBounds := walk.Rectangle{X: rect.X + dashboard.px(19), Y: rect.Y + dashboard.px(30), Width: dashboard.px(10), Height: dashboard.px(10)}
		dotBrush := dashboard.theme.faint
		if profile.State == manager.TunnelStarted {
			dotBrush = dashboard.theme.success
		} else if profile.State == manager.TunnelStarting || profile.State == manager.TunnelStopping {
			dotBrush = dashboard.theme.warning
		}
		dashboard.fillEllipse(canvas, dotBrush, dotBounds)
		dashboard.drawText(canvas, profile.Tunnel.Name, dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: rect.X + dashboard.px(44), Y: rect.Y + dashboard.px(10), Width: rect.Width - dashboard.px(310), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		dashboard.drawText(canvas, profile.Endpoint, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: rect.X + dashboard.px(44), Y: rect.Y + dashboard.px(37), Width: rect.Width - dashboard.px(310), Height: dashboard.px(22)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		statusBounds := walk.Rectangle{X: rect.X + rect.Width - dashboard.px(248), Y: rect.Y + dashboard.px(23), Width: dashboard.px(106), Height: dashboard.px(26)}
		dashboard.fillRounded(canvas, dashboard.theme.sidebar, statusBounds, dashboard.px(13))
		dashboard.drawText(canvas, stateText, dashboard.theme.microFont, stateColor, statusBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
		dashboard.drawIconButton(canvas, fmt.Sprintf("profile:edit:%d", index), "\ue70f", walk.Rectangle{X: rect.X + rect.Width - dashboard.px(96), Y: rect.Y + dashboard.px(18), Width: dashboard.px(38), Height: dashboard.px(36)}, false)
		dashboard.drawIconButton(canvas, fmt.Sprintf("profile:delete:%d", index), "\ue74d", walk.Rectangle{X: rect.X + rect.Width - dashboard.px(48), Y: rect.Y + dashboard.px(18), Width: dashboard.px(38), Height: dashboard.px(36)}, true)
	}
	dashboard.drawScrollIndicator(canvas, walk.Rectangle{X: content.X, Y: listY, Width: content.Width, Height: visibleHeight}, len(dashboard.profiles), visibleCount, dashboard.profileScroll)
}

func (dashboard *Dashboard) drawDiagnosticsPage(canvas *walk.Canvas, content walk.Rectangle) {
	startY := content.Y + dashboard.px(76)
	profileName := "Не выбран"
	endpoint := "—"
	if profile := dashboard.selectedProfile(); profile != nil {
		profileName = profile.Tunnel.Name
		endpoint = profile.Endpoint
	}
	modeTitle, _ := modeCopy(dashboard.settings.Mode)
	status, _, statusColor := stateCopy(dashboard.globalState, dashboard.operationBusy())
	rows := []struct {
		label string
		value string
		color walk.Color
	}{
		{"Состояние", status, statusColor},
		{"Текущий профиль", profileName, dashboard.theme.textColor},
		{"Сервер", endpoint, dashboard.theme.textColor},
		{"Маршрутизация", modeTitle, dashboard.theme.textColor},
		{"Пользовательских правил", fmt.Sprintf("%d", len(dashboard.settings.CustomRules)), dashboard.theme.textColor},
	}
	card := walk.Rectangle{X: content.X, Y: startY, Width: content.Width, Height: dashboard.px(250)}
	dashboard.drawCard(canvas, "", card, false, false)
	rowY := card.Y + dashboard.px(18)
	for _, row := range rows {
		dashboard.drawText(canvas, row.label, dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: card.X + dashboard.px(20), Y: rowY, Width: card.Width/2 - dashboard.px(30), Height: dashboard.px(28)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
		dashboard.drawText(canvas, row.value, dashboard.theme.bodyFont, row.color, walk.Rectangle{X: card.X + card.Width/2, Y: rowY, Width: card.Width/2 - dashboard.px(20), Height: dashboard.px(28)}, walk.TextRight|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
		rowY += dashboard.px(42)
	}

	buttonY := card.Y + card.Height + dashboard.px(18)
	gap := dashboard.px(12)
	buttonWidth := (content.Width - gap*2) / 3
	dashboard.drawButton(canvas, "diagnostics:folder", "Папка логов", "\ue8b7", walk.Rectangle{X: content.X, Y: buttonY, Width: buttonWidth, Height: dashboard.px(42)}, false, false)
	dashboard.drawButton(canvas, "diagnostics:copy", "Копировать", "\ue8c8", walk.Rectangle{X: content.X + buttonWidth + gap, Y: buttonY, Width: buttonWidth, Height: dashboard.px(42)}, false, false)
	dashboard.drawButton(canvas, "diagnostics:about", "О приложении", "\ue946", walk.Rectangle{X: content.X + (buttonWidth+gap)*2, Y: buttonY, Width: buttonWidth, Height: dashboard.px(42)}, false, false)

	privacy := walk.Rectangle{X: content.X, Y: buttonY + dashboard.px(66), Width: content.Width, Height: dashboard.px(74)}
	dashboard.drawCard(canvas, "", privacy, false, false)
	dashboard.drawText(canvas, "Приватность", dashboard.theme.headingFont, dashboard.theme.textColor, walk.Rectangle{X: privacy.X + dashboard.px(18), Y: privacy.Y + dashboard.px(10), Width: privacy.Width - dashboard.px(36), Height: dashboard.px(24)}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine)
	dashboard.drawText(canvas, "Диагностика не копирует приватные и preshared-ключи. Профили и правила хранятся только локально.", dashboard.theme.smallFont, dashboard.theme.mutedColor, walk.Rectangle{X: privacy.X + dashboard.px(18), Y: privacy.Y + dashboard.px(35), Width: privacy.Width - dashboard.px(36), Height: dashboard.px(28)}, walk.TextLeft|walk.TextVCenter|walk.TextWordbreak)
}

func (dashboard *Dashboard) drawErrorBanner(canvas *walk.Canvas, bounds walk.Rectangle, navWidth int) {
	banner := walk.Rectangle{X: navWidth + dashboard.px(20), Y: bounds.Height - dashboard.px(58), Width: bounds.Width - navWidth - dashboard.px(40), Height: dashboard.px(42)}
	dashboard.fillBorderedRounded(canvas, dashboard.theme.surface, dashboard.theme.danger, banner, dashboard.px(7), dashboard.px(2))
	if dashboard.smooth == nil {
		dashboard.drawRounded(canvas, dashboard.theme.dangerPen, banner, dashboard.px(7))
	}
	dashboard.drawText(canvas, "\ue783", dashboard.theme.iconFont, dashboard.theme.dangerColor, walk.Rectangle{X: banner.X + dashboard.px(12), Y: banner.Y, Width: dashboard.px(28), Height: banner.Height}, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	message := strings.ReplaceAll(dashboard.lastError, "\n", " ")
	dashboard.drawText(canvas, message, dashboard.theme.smallFont, dashboard.theme.textColor, walk.Rectangle{X: banner.X + dashboard.px(46), Y: banner.Y, Width: banner.Width - dashboard.px(88), Height: banner.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	closeBounds := walk.Rectangle{X: banner.X + banner.Width - dashboard.px(38), Y: banner.Y + dashboard.px(6), Width: dashboard.px(30), Height: dashboard.px(30)}
	dashboard.drawText(canvas, "\ue711", dashboard.theme.iconFont, dashboard.theme.mutedColor, closeBounds, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
	dashboard.addHit("error:dismiss", closeBounds, false)
}
