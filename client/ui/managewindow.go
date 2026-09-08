/* SPDX-License-Identifier: MIT */

package ui

import (
	"sync"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"github.com/amnezia-vpn/amneziawg-windows-client/manager"
)

type ManageTunnelsWindow struct {
	walk.FormBase

	dashboard *Dashboard
	preview   bool

	tunnelChangedCB *manager.TunnelChangeCallback
}

const (
	manageWindowWindowClass = "Pinus Smart AWG Preview UI"
	raiseMsg                = win.WM_USER + 0x3510
	aboutWireGuardCmd       = 0x37
)

var taskbarButtonCreatedMsg uint32
var initedManageTunnels sync.Once

func NewManageTunnelsWindow(preview bool) (*ManageTunnelsWindow, error) {
	initedManageTunnels.Do(func() {
		walk.AppendToWalkInit(func() {
			walk.MustRegisterWindowClass(manageWindowWindowClass)
			taskbarButtonCreatedMsg = win.RegisterWindowMessage(windows.StringToUTF16Ptr("TaskbarButtonCreated"))
		})
	})

	var disposables walk.Disposables
	defer disposables.Treat()

	font, err := walk.NewFont("Verdana", 9, 0)
	if err != nil {
		return nil, err
	}

	window := &ManageTunnelsWindow{preview: preview}
	window.SetName("PinusSmartAWGPreview")
	windowStyle := uint32(win.WS_OVERLAPPEDWINDOW | win.WS_CLIPCHILDREN)
	windowExStyle := uint32(win.WS_EX_CONTROLPARENT)
	if err = walk.InitWindow(window, nil, manageWindowWindowClass, windowStyle, windowExStyle); err != nil {
		font.Dispose()
		return nil, err
	}
	applyWindowChrome(window.Handle())
	disposables.Add(window)
	window.AddDisposable(font)
	win.ChangeWindowMessageFilterEx(window.Handle(), raiseMsg, win.MSGFLT_ALLOW, nil)
	window.SetPersistent(!preview)

	if icon, iconErr := loadLogoIcon(32); iconErr == nil {
		window.SetIcon(icon)
	}
	title := "Pinus Smart AWG Preview 3.3.0"
	if preview {
		title += " — UI Preview"
	}
	window.SetTitle(title)
	window.SetFont(font)
	window.SetSize(walk.Size{1040, 700})
	window.SetMinMaxSize(walk.Size{900, 620}, walk.Size{0, 0})
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{})
	layout.SetSpacing(0)
	window.SetLayout(layout)

	window.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if preview {
			return
		}
		*canceled = true
		if !noTrayAvailable {
			window.Hide()
		} else {
			win.ShowWindow(window.Handle(), win.SW_MINIMIZE)
		}
	})
	window.VisibleChanged().Attach(func() {
		if window.Visible() {
			window.dashboard.SelectActiveProfile()
			win.SetForegroundWindow(window.Handle())
			win.BringWindowToTop(window.Handle())
		}
	})

	if window.dashboard, err = NewDashboard(window, preview); err != nil {
		return nil, err
	}
	if !preview {
		window.tunnelChangedCB = manager.IPCClientRegisterTunnelChange(window.onTunnelChange)
		globalState, _ := manager.IPCClientGlobalState()
		window.onTunnelChange(nil, manager.TunnelUnknown, globalState, nil)
	}

	systemMenu := win.GetSystemMenu(window.Handle(), false)
	if systemMenu != 0 {
		win.InsertMenuItem(systemMenu, 0, true, &win.MENUITEMINFO{
			CbSize:     uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
			FMask:      win.MIIM_ID | win.MIIM_STRING | win.MIIM_FTYPE,
			FType:      win.MIIM_STRING,
			DwTypeData: windows.StringToUTF16Ptr("О Pinus Smart AWG..."),
			WID:        uint32(aboutWireGuardCmd),
		})
		win.InsertMenuItem(systemMenu, 1, true, &win.MENUITEMINFO{
			CbSize: uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
			FMask:  win.MIIM_TYPE,
			FType:  win.MFT_SEPARATOR,
		})
	}

	disposables.Spare()
	return window, nil
}

func applyWindowChrome(hwnd win.HWND) {
	darkMode := int32(1)
	_ = windows.DwmSetWindowAttribute(windows.HWND(hwnd), windows.DWMWA_USE_IMMERSIVE_DARK_MODE, unsafe.Pointer(&darkMode), uint32(unsafe.Sizeof(darkMode)))
	roundCorners := int32(2)
	_ = windows.DwmSetWindowAttribute(windows.HWND(hwnd), windows.DWMWA_WINDOW_CORNER_PREFERENCE, unsafe.Pointer(&roundCorners), uint32(unsafe.Sizeof(roundCorners)))
	borderColor := uint32(walk.RGB(34, 67, 99))
	_ = windows.DwmSetWindowAttribute(windows.HWND(hwnd), windows.DWMWA_BORDER_COLOR, unsafe.Pointer(&borderColor), uint32(unsafe.Sizeof(borderColor)))
}

func resizeHitTest(hwnd win.HWND, lParam uintptr) uintptr {
	if win.IsZoomed(hwnd) {
		return win.HTCLIENT
	}
	var bounds win.RECT
	if !win.GetWindowRect(hwnd, &bounds) {
		return win.HTCLIENT
	}
	x := int(int16(lParam & 0xffff))
	y := int(int16((lParam >> 16) & 0xffff))
	dpi := win.GetDpiForWindow(hwnd)
	borderX := int(win.GetSystemMetricsForDpi(win.SM_CXSIZEFRAME, dpi))
	borderY := int(win.GetSystemMetricsForDpi(win.SM_CYSIZEFRAME, dpi))
	left := x < int(bounds.Left)+borderX
	right := x >= int(bounds.Right)-borderX
	top := y < int(bounds.Top)+borderY
	bottom := y >= int(bounds.Bottom)-borderY
	switch {
	case top && left:
		return win.HTTOPLEFT
	case top && right:
		return win.HTTOPRIGHT
	case bottom && left:
		return win.HTBOTTOMLEFT
	case bottom && right:
		return win.HTBOTTOMRIGHT
	case left:
		return win.HTLEFT
	case right:
		return win.HTRIGHT
	case top:
		return win.HTTOP
	case bottom:
		return win.HTBOTTOM
	default:
		return win.HTCLIENT
	}
}

func (window *ManageTunnelsWindow) Dispose() {
	if window.tunnelChangedCB != nil {
		window.tunnelChangedCB.Unregister()
		window.tunnelChangedCB = nil
	}
	window.FormBase.Dispose()
}

func (window *ManageTunnelsWindow) updateProgressIndicator(globalState manager.TunnelState) {
	indicator := window.ProgressIndicator()
	if indicator == nil {
		return
	}
	if globalState == manager.TunnelStopping || globalState == manager.TunnelStarting {
		indicator.SetState(walk.PIIndeterminate)
	} else {
		indicator.SetState(walk.PINoProgress)
	}
	if icon, err := iconForState(globalState, 16); err == nil {
		if globalState == manager.TunnelStopped {
			icon = nil
		}
		indicator.SetOverlayIcon(icon, textForState(globalState, false))
	}
}

func (window *ManageTunnelsWindow) onTunnelChange(_ *manager.Tunnel, _ manager.TunnelState, globalState manager.TunnelState, _ error) {
	window.Synchronize(func() {
		window.updateProgressIndicator(globalState)
	})
}

func (window *ManageTunnelsWindow) UpdateFound() {
	// This fork ships without the upstream updater. Keep the method for tray/API compatibility.
}

func (window *ManageTunnelsWindow) WndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_NCCALCSIZE:
		// Preserve normal DWM behavior while letting the dashboard own the
		// entire visible frame and title bar.
		return 0
	case win.WM_NCHITTEST:
		if hit := resizeHitTest(hwnd, lParam); hit != win.HTCLIENT {
			return hit
		}
	case win.WM_QUERYENDSESSION:
		if lParam == win.ENDSESSION_CLOSEAPP {
			return win.TRUE
		}
	case win.WM_ENDSESSION:
		if lParam == win.ENDSESSION_CLOSEAPP && wParam == 1 {
			walk.App().Exit(198)
		}
	case win.WM_SYSCOMMAND:
		if wParam == aboutWireGuardCmd {
			onAbout(window)
			return 0
		}
	case raiseMsg:
		if window.dashboard == nil {
			window.Synchronize(func() { window.SendMessage(msg, wParam, lParam) })
			return 0
		}
		window.dashboard.SelectActiveProfile()
		window.dashboard.ShowPage(pageHome)
		raise(window.Handle())
		return 0
	case taskbarButtonCreatedMsg:
		result := window.FormBase.WndProc(hwnd, msg, wParam, lParam)
		if !window.preview {
			go func() {
				globalState, err := manager.IPCClientGlobalState()
				if err == nil {
					window.Synchronize(func() { window.updateProgressIndicator(globalState) })
				}
			}()
		}
		return result
	}
	return window.FormBase.WndProc(hwnd, msg, wParam, lParam)
}
