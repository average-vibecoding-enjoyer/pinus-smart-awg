/* SPDX-License-Identifier: MIT */

package ui

import (
	"sync"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

type gdiplusStartupInput struct {
	Version                  uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

type smoothCanvas struct {
	hdc      win.HDC
	graphics uintptr
}

type gdiplusRect struct {
	X      int32
	Y      int32
	Width  int32
	Height int32
}

var (
	gdiplusDLL = windows.NewLazySystemDLL("gdiplus.dll")

	gdiplusStartup            = gdiplusDLL.NewProc("GdiplusStartup")
	gdipCreateFromHDC         = gdiplusDLL.NewProc("GdipCreateFromHDC")
	gdipDeleteGraphics        = gdiplusDLL.NewProc("GdipDeleteGraphics")
	gdipSetSmoothingMode      = gdiplusDLL.NewProc("GdipSetSmoothingMode")
	gdipSetPixelOffsetMode    = gdiplusDLL.NewProc("GdipSetPixelOffsetMode")
	gdipSetCompositingQuality = gdiplusDLL.NewProc("GdipSetCompositingQuality")
	gdipFlush                 = gdiplusDLL.NewProc("GdipFlush")
	gdipCreateSolidFill       = gdiplusDLL.NewProc("GdipCreateSolidFill")
	gdipCreateLineBrush       = gdiplusDLL.NewProc("GdipCreateLineBrushFromRectI")
	gdipDeleteBrush           = gdiplusDLL.NewProc("GdipDeleteBrush")
	gdipFillRectangleI        = gdiplusDLL.NewProc("GdipFillRectangleI")
	gdipFillEllipseI          = gdiplusDLL.NewProc("GdipFillEllipseI")
	gdipCreatePath            = gdiplusDLL.NewProc("GdipCreatePath")
	gdipDeletePath            = gdiplusDLL.NewProc("GdipDeletePath")
	gdipAddPathLineI          = gdiplusDLL.NewProc("GdipAddPathLineI")
	gdipAddPathBezierI        = gdiplusDLL.NewProc("GdipAddPathBezierI")
	gdipClosePathFigure       = gdiplusDLL.NewProc("GdipClosePathFigure")
	gdipFillPath              = gdiplusDLL.NewProc("GdipFillPath")
	gdiplusInitializeOnce     sync.Once
	gdiplusAvailable          bool
	gdiplusToken              uintptr
)

func initializeGDIPlus() bool {
	gdiplusInitializeOnce.Do(func() {
		input := gdiplusStartupInput{Version: 1}
		status, _, _ := gdiplusStartup.Call(
			uintptr(unsafe.Pointer(&gdiplusToken)),
			uintptr(unsafe.Pointer(&input)),
			0,
		)
		gdiplusAvailable = status == 0
	})
	return gdiplusAvailable
}

func newSmoothCanvas(hdc win.HDC) *smoothCanvas {
	if hdc == 0 || !initializeGDIPlus() {
		return nil
	}
	return &smoothCanvas{hdc: hdc}
}

func (canvas *smoothCanvas) ensureGraphics() bool {
	if canvas == nil || canvas.hdc == 0 {
		return false
	}
	if canvas.graphics != 0 {
		return true
	}
	var graphics uintptr
	status, _, _ := gdipCreateFromHDC.Call(uintptr(canvas.hdc), uintptr(unsafe.Pointer(&graphics)))
	if status != 0 || graphics == 0 {
		return false
	}
	// GDI+ AntiAlias and Half pixel offset remove the one-pixel stair steps
	// that plain GDI leaves on circles and rounded controls.
	gdipSetSmoothingMode.Call(graphics, 4)
	gdipSetPixelOffsetMode.Call(graphics, 4)
	gdipSetCompositingQuality.Call(graphics, 2)
	canvas.graphics = graphics
	return true
}

func (canvas *smoothCanvas) releaseGraphics() {
	if canvas == nil || canvas.graphics == 0 {
		return
	}
	gdipFlush.Call(canvas.graphics, 1)
	gdipDeleteGraphics.Call(canvas.graphics)
	canvas.graphics = 0
}

func (canvas *smoothCanvas) Dispose() {
	canvas.releaseGraphics()
	if canvas != nil {
		canvas.hdc = 0
	}
}

func (canvas *smoothCanvas) Flush() {
	// GDI and GDI+ must not draw into the same HDC concurrently. Releasing the
	// graphics object here makes subsequent Walk text/image operations stable.
	canvas.releaseGraphics()
}

func colorARGB(color walk.Color) uint32 {
	red := uint32(color) & 0xff
	green := (uint32(color) >> 8) & 0xff
	blue := (uint32(color) >> 16) & 0xff
	return 0xff000000 | red<<16 | green<<8 | blue
}

func (canvas *smoothCanvas) withBrush(color walk.Color, draw func(brush uintptr) bool) bool {
	if !canvas.ensureGraphics() {
		return false
	}
	var brush uintptr
	status, _, _ := gdipCreateSolidFill.Call(uintptr(colorARGB(color)), uintptr(unsafe.Pointer(&brush)))
	if status != 0 || brush == 0 {
		return false
	}
	defer gdipDeleteBrush.Call(brush)
	return draw(brush)
}

func (canvas *smoothCanvas) withGradientBrush(bounds walk.Rectangle, start, end walk.Color, direction int, draw func(brush uintptr) bool) bool {
	if !canvas.ensureGraphics() || bounds.Width <= 0 || bounds.Height <= 0 {
		return false
	}
	rect := gdiplusRect{X: int32(bounds.X), Y: int32(bounds.Y), Width: int32(bounds.Width), Height: int32(bounds.Height)}
	var brush uintptr
	status, _, _ := gdipCreateLineBrush.Call(
		uintptr(unsafe.Pointer(&rect)),
		uintptr(colorARGB(start)),
		uintptr(colorARGB(end)),
		uintptr(direction),
		0,
		uintptr(unsafe.Pointer(&brush)),
	)
	if status != 0 || brush == 0 {
		return false
	}
	defer gdipDeleteBrush.Call(brush)
	return draw(brush)
}

func (canvas *smoothCanvas) fillRectangleWithBrush(brush uintptr, bounds walk.Rectangle) bool {
	status, _, _ := gdipFillRectangleI.Call(
		canvas.graphics,
		brush,
		uintptr(bounds.X),
		uintptr(bounds.Y),
		uintptr(bounds.Width),
		uintptr(bounds.Height),
	)
	return status == 0
}

func (canvas *smoothCanvas) fillEllipseWithBrush(brush uintptr, bounds walk.Rectangle) bool {
	status, _, _ := gdipFillEllipseI.Call(
		canvas.graphics,
		brush,
		uintptr(bounds.X),
		uintptr(bounds.Y),
		uintptr(bounds.Width),
		uintptr(bounds.Height),
	)
	return status == 0
}

func (canvas *smoothCanvas) FillRectangle(color walk.Color, bounds walk.Rectangle) bool {
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return false
	}
	return canvas.withBrush(color, func(brush uintptr) bool {
		return canvas.fillRectangleWithBrush(brush, bounds)
	})
}

func (canvas *smoothCanvas) FillRectangleGradient(bounds walk.Rectangle, start, end walk.Color, direction int) bool {
	return canvas.withGradientBrush(bounds, start, end, direction, func(brush uintptr) bool {
		return canvas.fillRectangleWithBrush(brush, bounds)
	})
}

func (canvas *smoothCanvas) FillEllipse(color walk.Color, bounds walk.Rectangle) bool {
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return false
	}
	return canvas.withBrush(color, func(brush uintptr) bool {
		return canvas.fillEllipseWithBrush(brush, bounds)
	})
}

func (canvas *smoothCanvas) FillEllipseGradient(bounds walk.Rectangle, start, end walk.Color, direction int) bool {
	return canvas.withGradientBrush(bounds, start, end, direction, func(brush uintptr) bool {
		return canvas.fillEllipseWithBrush(brush, bounds)
	})
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func (canvas *smoothCanvas) fillRoundedRectangleWithBrush(brush uintptr, bounds walk.Rectangle, radius int) bool {
	if bounds.Width <= 0 || bounds.Height <= 0 {
		return false
	}
	if radius <= 0 {
		return canvas.fillRectangleWithBrush(brush, bounds)
	}
	maximumRadius := bounds.Width / 2
	if bounds.Height/2 < maximumRadius {
		maximumRadius = bounds.Height / 2
	}
	radius = clamp(radius, 1, maximumRadius)
	if bounds.Width == bounds.Height && radius == maximumRadius {
		return canvas.fillEllipseWithBrush(brush, bounds)
	}

	var path uintptr
	status, _, _ := gdipCreatePath.Call(0, uintptr(unsafe.Pointer(&path)))
	if status != 0 || path == 0 {
		return false
	}
	defer gdipDeletePath.Call(path)

	x := bounds.X
	y := bounds.Y
	right := bounds.X + bounds.Width
	bottom := bounds.Y + bounds.Height
	// Cubic approximation of a quarter circle. All calls use the integer GDI+
	// API, which keeps the syscall ABI predictable while smoothing the result.
	handle := int(float64(radius)*0.5522847498 + 0.5)
	addLine := func(x1, y1, x2, y2 int) bool {
		result, _, _ := gdipAddPathLineI.Call(path, uintptr(x1), uintptr(y1), uintptr(x2), uintptr(y2))
		return result == 0
	}
	addBezier := func(x1, y1, x2, y2, x3, y3, x4, y4 int) bool {
		result, _, _ := gdipAddPathBezierI.Call(
			path,
			uintptr(x1), uintptr(y1),
			uintptr(x2), uintptr(y2),
			uintptr(x3), uintptr(y3),
			uintptr(x4), uintptr(y4),
		)
		return result == 0
	}

	if !addLine(x+radius, y, right-radius, y) ||
		!addBezier(right-radius, y, right-radius+handle, y, right, y+radius-handle, right, y+radius) ||
		!addLine(right, y+radius, right, bottom-radius) ||
		!addBezier(right, bottom-radius, right, bottom-radius+handle, right-radius+handle, bottom, right-radius, bottom) ||
		!addLine(right-radius, bottom, x+radius, bottom) ||
		!addBezier(x+radius, bottom, x+radius-handle, bottom, x, bottom-radius+handle, x, bottom-radius) ||
		!addLine(x, bottom-radius, x, y+radius) ||
		!addBezier(x, y+radius, x, y+radius-handle, x+radius-handle, y, x+radius, y) {
		return false
	}
	status, _, _ = gdipClosePathFigure.Call(path)
	if status != 0 {
		return false
	}
	result, _, _ := gdipFillPath.Call(canvas.graphics, brush, path)
	return result == 0
}

func (canvas *smoothCanvas) FillRoundedRectangle(color walk.Color, bounds walk.Rectangle, radius int) bool {
	return canvas.withBrush(color, func(brush uintptr) bool {
		return canvas.fillRoundedRectangleWithBrush(brush, bounds, radius)
	})
}

func (canvas *smoothCanvas) FillRoundedGradient(bounds walk.Rectangle, radius int, start, end walk.Color, direction int) bool {
	return canvas.withGradientBrush(bounds, start, end, direction, func(brush uintptr) bool {
		return canvas.fillRoundedRectangleWithBrush(brush, bounds, radius)
	})
}
