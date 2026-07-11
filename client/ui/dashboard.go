/* SPDX-License-Identifier: MIT */

package ui

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"

	"github.com/amnezia-vpn/amneziawg-windows-client/manager"
	clientservices "github.com/amnezia-vpn/amneziawg-windows-client/services"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

type dashboardPage int

const (
	pageHome dashboardPage = iota
	pageRouting
	pageRules
	pageProfiles
	pageDiagnostics
)

type profileInfo struct {
	Tunnel   manager.Tunnel
	Endpoint string
	State    manager.TunnelState
}

type dashboardHit struct {
	ID       string
	Bounds   walk.Rectangle
	Disabled bool
}

type Dashboard struct {
	*walk.CustomWidget

	preview       bool
	theme         *dashboardTheme
	smooth        *smoothCanvas
	backing       *walk.Bitmap
	backingCanvas *walk.Canvas
	backingSize   walk.Size
	backingDPI    int
	queueGDI      bool
	gdiCommands   []dashboardGDICommand
	page          dashboardPage

	profiles      []profileInfo
	selected      int
	activeName    string
	settings      smart.RoutingSettings
	globalState   manager.TunnelState
	connectedAt   time.Time
	lastError     string
	operation     uint32
	hoverID       string
	pressedID     string
	hits          []dashboardHit
	profileScroll int
	ruleScroll    int
	animation     float64

	tunnelChangedCB  *manager.TunnelChangeCallback
	tunnelsChangedCB *manager.TunnelsChangeCallback
	stopAnimation    chan struct{}
	disposeOnce      sync.Once
}

func NewDashboard(parent walk.Container, preview bool) (*Dashboard, error) {
	theme, err := newDashboardTheme()
	if err != nil {
		return nil, err
	}

	dashboard := &Dashboard{
		preview:       preview,
		theme:         theme,
		page:          pageHome,
		selected:      -1,
		settings:      smart.DefaultSettings(),
		globalState:   manager.TunnelStopped,
		stopAnimation: make(chan struct{}),
	}
	widget, err := walk.NewCustomWidgetPixels(parent, win.WS_TABSTOP, dashboard.paint)
	if err != nil {
		theme.Dispose()
		return nil, err
	}
	dashboard.CustomWidget = widget
	// GDI+ must draw against the real widget HDC. Walk's partial-size memory
	// buffer applies a viewport transform that GDI+ does not consistently honor.
	dashboard.SetPaintMode(walk.PaintNoErase)
	dashboard.SetInvalidatesOnResize(false)
	dashboard.SetCursor(walk.CursorArrow())
	dashboard.SetToolTipText("")

	dashboard.MouseMove().Attach(dashboard.onMouseMove)
	dashboard.MouseDown().Attach(dashboard.onMouseDown)
	dashboard.MouseUp().Attach(dashboard.onMouseUp)
	dashboard.MouseWheel().Attach(dashboard.onMouseWheel)
	dashboard.KeyDown().Attach(dashboard.onKeyDown)
	dashboard.SizeChanged().Attach(func() { _ = dashboard.Invalidate() })
	dashboard.Disposing().Attach(func() { dashboard.stop() })

	if preview {
		dashboard.loadPreviewProfiles()
	} else {
		dashboard.loadProfiles()
		dashboard.tunnelChangedCB = manager.IPCClientRegisterTunnelChange(dashboard.onTunnelChanged)
		dashboard.tunnelsChangedCB = manager.IPCClientRegisterTunnelsChange(dashboard.onTunnelsChanged)
	}
	dashboard.startAnimationLoop()
	return dashboard, nil
}

// Invalidate keeps the last complete frame visible until the next back buffer
// is ready. Walk's default implementation requests an immediate background
// erase, which produces a visible dark flash between mouse events.
func (dashboard *Dashboard) Invalidate() error {
	if dashboard.CustomWidget == nil || dashboard.Handle() == 0 {
		return nil
	}
	if !win.InvalidateRect(dashboard.Handle(), nil, false) {
		return errors.New("invalidate dashboard")
	}
	return nil
}

func (dashboard *Dashboard) stop() {
	dashboard.disposeOnce.Do(func() {
		close(dashboard.stopAnimation)
		if dashboard.tunnelChangedCB != nil {
			dashboard.tunnelChangedCB.Unregister()
			dashboard.tunnelChangedCB = nil
		}
		if dashboard.tunnelsChangedCB != nil {
			dashboard.tunnelsChangedCB.Unregister()
			dashboard.tunnelsChangedCB = nil
		}
		if dashboard.theme != nil {
			dashboard.theme.Dispose()
			dashboard.theme = nil
		}
		if dashboard.backingCanvas != nil {
			dashboard.backingCanvas.Dispose()
			dashboard.backingCanvas = nil
		}
		if dashboard.backing != nil {
			dashboard.backing.Dispose()
			dashboard.backing = nil
		}
	})
}

func (dashboard *Dashboard) startAnimationLoop() {
	go func() {
		ticker := time.NewTicker(time.Second / 30)
		defer ticker.Stop()
		started := time.Now()
		tick := 0
		for {
			select {
			case <-dashboard.stopAnimation:
				return
			case <-ticker.C:
				if dashboard.CustomWidget == nil || dashboard.Handle() == 0 || !win.IsWindowVisible(dashboard.Handle()) {
					continue
				}
				tick++
				dashboard.Synchronize(func() {
					dashboard.animation = time.Since(started).Seconds()
					animatedState := atomic.LoadUint32(&dashboard.operation) != 0 || dashboard.globalState == manager.TunnelStarted || dashboard.globalState == manager.TunnelStarting || dashboard.globalState == manager.TunnelStopping
					if dashboard.page == pageHome && animatedState || tick%30 == 0 {
						_ = dashboard.Invalidate()
					}
				})
			}
		}
	}()
}

func (dashboard *Dashboard) loadPreviewProfiles() {
	dashboard.profiles = []profileInfo{
		{Tunnel: manager.Tunnel{Name: "Pinus Warsaw"}, Endpoint: "pl-waw.pin.us:443", State: manager.TunnelStopped},
		{Tunnel: manager.Tunnel{Name: "Pinus Helsinki"}, Endpoint: "fi-hel.pin.us:443", State: manager.TunnelStopped},
	}
	dashboard.selected = 0
	dashboard.settings = smart.DefaultSettings()
}

func endpointForTunnel(tunnel manager.Tunnel) string {
	config, err := tunnel.StoredConfig()
	if err != nil || len(config.Peers) == 0 || config.Peers[0].Endpoint.IsEmpty() {
		return "Адрес сервера скрыт"
	}
	endpoint := config.Peers[0].Endpoint
	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
}

func (dashboard *Dashboard) loadProfiles() {
	selectedName := clientservices.UserKeyString("SelectedProfile")
	if profile := dashboard.selectedProfile(); profile != nil {
		selectedName = profile.Tunnel.Name
	}
	tunnels, err := manager.IPCClientTunnels()
	if err != nil {
		dashboard.lastError = err.Error()
		return
	}
	sort.SliceStable(tunnels, func(i, j int) bool {
		return conf.TunnelNameIsLess(tunnels[i].Name, tunnels[j].Name)
	})
	profiles := make([]profileInfo, 0, len(tunnels))
	activeName := ""
	for _, tunnel := range tunnels {
		state, stateErr := tunnel.State()
		if stateErr != nil {
			state = manager.TunnelUnknown
		}
		if state == manager.TunnelStarted || state == manager.TunnelStarting {
			activeName = tunnel.Name
		}
		profiles = append(profiles, profileInfo{
			Tunnel:   tunnel,
			Endpoint: endpointForTunnel(tunnel),
			State:    state,
		})
	}
	dashboard.profiles = profiles
	dashboard.activeName = activeName
	dashboard.selected = -1
	if activeName != "" {
		selectedName = activeName
	}
	for i := range dashboard.profiles {
		if dashboard.profiles[i].Tunnel.Name == selectedName {
			dashboard.selected = i
			break
		}
	}
	if dashboard.selected < 0 && len(dashboard.profiles) > 0 {
		dashboard.selected = 0
	}
	if dashboard.selected >= 0 {
		_ = clientservices.SetUserKeyString("SelectedProfile", dashboard.profiles[dashboard.selected].Tunnel.Name)
		settings, settingsErr := smart.LoadSettings(dashboard.profiles[dashboard.selected].Tunnel.Name)
		if settingsErr != nil {
			dashboard.lastError = settingsErr.Error()
			settings = smart.DefaultSettings()
		}
		dashboard.settings = settings
	} else {
		dashboard.settings = smart.DefaultSettings()
	}
	if state, stateErr := manager.IPCClientGlobalState(); stateErr == nil {
		dashboard.globalState = state
	}
	if dashboard.globalState == manager.TunnelStarted && dashboard.connectedAt.IsZero() {
		dashboard.connectedAt = time.Now()
	}
}

func (dashboard *Dashboard) selectedProfile() *profileInfo {
	if dashboard.selected < 0 || dashboard.selected >= len(dashboard.profiles) {
		return nil
	}
	return &dashboard.profiles[dashboard.selected]
}

func (dashboard *Dashboard) SelectActiveProfile() {
	for i := range dashboard.profiles {
		if dashboard.profiles[i].State == manager.TunnelStarted || dashboard.profiles[i].State == manager.TunnelStarting {
			dashboard.selectProfileWithActivation(i, false)
			return
		}
	}
}

func (dashboard *Dashboard) SelectProfileName(name string) {
	for i := range dashboard.profiles {
		if dashboard.profiles[i].Tunnel.Name == name {
			dashboard.selectProfileWithActivation(i, false)
			return
		}
	}
}

func (dashboard *Dashboard) ShowPage(page dashboardPage) {
	dashboard.page = page
	dashboard.hoverID = ""
	dashboard.profileScroll = 0
	dashboard.ruleScroll = 0
	_ = dashboard.Invalidate()
}

func (dashboard *Dashboard) onTunnelChanged(tunnel *manager.Tunnel, state, globalState manager.TunnelState, changeErr error) {
	dashboard.Synchronize(func() {
		dashboard.globalState = globalState
		if tunnel != nil {
			for i := range dashboard.profiles {
				if dashboard.profiles[i].Tunnel.Name == tunnel.Name {
					dashboard.profiles[i].State = state
					break
				}
			}
			if state == manager.TunnelStarted || state == manager.TunnelStarting {
				dashboard.activeName = tunnel.Name
			}
			if state == manager.TunnelStopped && dashboard.activeName == tunnel.Name && globalState == manager.TunnelStopped {
				dashboard.activeName = ""
			}
		}
		if globalState == manager.TunnelStarted && dashboard.connectedAt.IsZero() {
			dashboard.connectedAt = time.Now()
		}
		if globalState == manager.TunnelStopped {
			dashboard.connectedAt = time.Time{}
		}
		if changeErr != nil {
			dashboard.lastError = changeErr.Error()
		}
		_ = dashboard.Invalidate()
	})
}

func (dashboard *Dashboard) onTunnelsChanged() {
	dashboard.Synchronize(func() {
		dashboard.loadProfiles()
		_ = dashboard.Invalidate()
	})
}

func pointInside(x, y int, bounds walk.Rectangle) bool {
	return x >= bounds.X && y >= bounds.Y && x < bounds.X+bounds.Width && y < bounds.Y+bounds.Height
}

func (dashboard *Dashboard) hitAt(x, y int) dashboardHit {
	for i := len(dashboard.hits) - 1; i >= 0; i-- {
		if pointInside(x, y, dashboard.hits[i].Bounds) {
			return dashboard.hits[i]
		}
	}
	return dashboardHit{}
}

func (dashboard *Dashboard) onMouseMove(x, y int, _ walk.MouseButton) {
	hit := dashboard.hitAt(x, y)
	if hit.ID != dashboard.hoverID {
		dashboard.hoverID = hit.ID
		if hit.ID != "" && !hit.Disabled {
			dashboard.SetCursor(walk.CursorHand())
		} else {
			dashboard.SetCursor(walk.CursorArrow())
		}
		_ = dashboard.Invalidate()
	}
}

func (dashboard *Dashboard) onMouseDown(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	hit := dashboard.hitAt(x, y)
	if hit.Disabled {
		return
	}
	if hit.ID == "window:drag" {
		dashboard.pressedID = ""
		win.ReleaseCapture()
		win.SendMessage(dashboard.Form().Handle(), win.WM_NCLBUTTONDOWN, uintptr(win.HTCAPTION), 0)
		return
	}
	dashboard.pressedID = hit.ID
	_ = dashboard.SetFocus()
	_ = dashboard.Invalidate()
}

func (dashboard *Dashboard) onMouseUp(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	hit := dashboard.hitAt(x, y)
	pressed := dashboard.pressedID
	dashboard.pressedID = ""
	_ = dashboard.Invalidate()
	if pressed != "" && pressed == hit.ID && !hit.Disabled {
		dashboard.activate(pressed)
	}
}

func (dashboard *Dashboard) onMouseWheel(_, _ int, button walk.MouseButton) {
	delta := walk.MouseWheelEventDelta(button)
	if dashboard.page == pageProfiles {
		dashboard.profileScroll -= delta / 120
		if dashboard.profileScroll < 0 {
			dashboard.profileScroll = 0
		}
	} else if dashboard.page == pageRules {
		dashboard.ruleScroll -= delta / 120
		if dashboard.ruleScroll < 0 {
			dashboard.ruleScroll = 0
		}
	}
	_ = dashboard.Invalidate()
}

func (dashboard *Dashboard) onKeyDown(key walk.Key) {
	switch key {
	case walk.KeyEscape:
		dashboard.ShowPage(pageHome)
	case walk.KeySpace:
		if dashboard.page == pageHome {
			dashboard.toggleConnection()
		}
	case walk.KeyF5:
		if !dashboard.preview {
			dashboard.loadProfiles()
			_ = dashboard.Invalidate()
		}
	}
}

func parseActionIndex(id, prefix string) (int, bool) {
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	return value, err == nil
}

func (dashboard *Dashboard) activate(id string) {
	switch id {
	case "window:minimize":
		win.ShowWindow(dashboard.Form().Handle(), win.SW_MINIMIZE)
	case "window:maximize":
		if win.IsZoomed(dashboard.Form().Handle()) {
			win.ShowWindow(dashboard.Form().Handle(), win.SW_RESTORE)
		} else {
			win.ShowWindow(dashboard.Form().Handle(), win.SW_MAXIMIZE)
		}
		_ = dashboard.Invalidate()
	case "window:close":
		win.SendMessage(dashboard.Form().Handle(), win.WM_CLOSE, 0, 0)
	case "nav:home":
		dashboard.ShowPage(pageHome)
	case "nav:routing", "home:routing":
		dashboard.ShowPage(pageRouting)
	case "nav:rules", "routing:rules":
		dashboard.ShowPage(pageRules)
	case "nav:profiles", "home:profile":
		dashboard.ShowPage(pageProfiles)
	case "nav:diagnostics":
		dashboard.ShowPage(pageDiagnostics)
	case "power":
		dashboard.toggleConnection()
	case "rules:add":
		dashboard.editRule(-1)
	case "profiles:import":
		dashboard.ImportProfiles()
	case "profiles:buy":
		openPinusVPNBot(dashboard.Form())
	case "diagnostics:folder":
		dashboard.openRuntimeFolder()
	case "diagnostics:copy":
		dashboard.copyDiagnostics()
	case "diagnostics:about":
		onAbout(dashboard.Form())
	case "error:dismiss":
		dashboard.lastError = ""
		_ = dashboard.Invalidate()
	}
	if index, ok := parseActionIndex(id, "mode:"); ok {
		dashboard.setMode(smart.ModeFromIndex(index))
		return
	}
	if strings.HasPrefix(id, "service:") {
		dashboard.toggleService(strings.TrimPrefix(id, "service:"))
		return
	}
	if index, ok := parseActionIndex(id, "rule:toggle:"); ok {
		dashboard.toggleRule(index)
		return
	}
	if index, ok := parseActionIndex(id, "rule:edit:"); ok {
		dashboard.editRule(index)
		return
	}
	if index, ok := parseActionIndex(id, "rule:delete:"); ok {
		dashboard.deleteRule(index)
		return
	}
	if index, ok := parseActionIndex(id, "profile:select:"); ok {
		dashboard.selectProfile(index)
		return
	}
	if index, ok := parseActionIndex(id, "profile:edit:"); ok {
		dashboard.editProfile(index)
		return
	}
	if index, ok := parseActionIndex(id, "profile:delete:"); ok {
		dashboard.deleteProfile(index)
	}
}

func (dashboard *Dashboard) operationBusy() bool {
	return atomic.LoadUint32(&dashboard.operation) != 0 || dashboard.globalState == manager.TunnelStarting || dashboard.globalState == manager.TunnelStopping
}

func (dashboard *Dashboard) toggleConnection() {
	if dashboard.operationBusy() || dashboard.selectedProfile() == nil {
		return
	}
	if dashboard.globalState == manager.TunnelStarted || dashboard.activeName != "" {
		dashboard.stopConnection()
	} else {
		dashboard.startSelectedProfile()
	}
}

func hasSelectedVPNRoute(settings smart.RoutingSettings) bool {
	if len(settings.SelectedApps) > 0 {
		return true
	}
	for _, rule := range settings.CustomRules {
		if rule.Enabled && rule.Target == smart.TargetVPN {
			return true
		}
	}
	return false
}

func shouldDisconnectForRoutingSettings(settings smart.RoutingSettings) bool {
	return smart.NormalizeMode(string(settings.Mode)) == smart.ModeSelected && !hasSelectedVPNRoute(settings)
}

func serviceSelectionEnabled(mode smart.Mode) bool {
	return smart.NormalizeMode(string(mode)) == smart.ModeSelected
}

func cloneRoutingSettings(settings smart.RoutingSettings) smart.RoutingSettings {
	settings.SelectedApps = append([]string(nil), settings.SelectedApps...)
	settings.CustomRules = append([]smart.CustomRule(nil), settings.CustomRules...)
	return settings
}

func (dashboard *Dashboard) startSelectedProfile() {
	dashboard.startSelectedProfileWithRollback(nil)
}

func (dashboard *Dashboard) startSelectedProfileWithRollback(rollbackSettings *smart.RoutingSettings) {
	profile := dashboard.selectedProfile()
	if profile == nil {
		return
	}
	if smart.NormalizeMode(string(dashboard.settings.Mode)) == smart.ModeSelected && !hasSelectedVPNRoute(dashboard.settings) {
		dashboard.lastError = "В режиме «Только выбранное» включи хотя бы один сервис или правило через VPN."
		_ = dashboard.Invalidate()
		return
	}
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	name := profile.Tunnel.Name
	payload, err := smart.SettingsPayload(dashboard.settings)
	if err != nil {
		atomic.StoreUint32(&dashboard.operation, 0)
		dashboard.lastError = err.Error()
		_ = dashboard.Invalidate()
		return
	}
	dashboard.globalState = manager.TunnelStarting
	dashboard.lastError = ""
	_ = dashboard.Invalidate()

	if dashboard.preview {
		go func() {
			time.Sleep(650 * time.Millisecond)
			dashboard.Synchronize(func() {
				atomic.StoreUint32(&dashboard.operation, 0)
				dashboard.globalState = manager.TunnelStarted
				dashboard.activeName = name
				dashboard.connectedAt = time.Now()
				for i := range dashboard.profiles {
					if dashboard.profiles[i].Tunnel.Name == name {
						dashboard.profiles[i].State = manager.TunnelStarted
					} else {
						dashboard.profiles[i].State = manager.TunnelStopped
					}
				}
				_ = dashboard.Invalidate()
			})
		}()
		return
	}

	go func() {
		startErr := manager.IPCClientSmartStart(name, payload)
		recoveredState := manager.TunnelStopped
		if startErr != nil {
			if state, stateErr := manager.IPCClientGlobalState(); stateErr == nil {
				recoveredState = state
			}
		}
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			if startErr != nil {
				if rollbackSettings != nil {
					restored := cloneRoutingSettings(*rollbackSettings)
					dashboard.settings = restored
					if !dashboard.preview {
						if saveErr := smart.SaveSettings(name, restored); saveErr != nil {
							startErr = errors.Join(startErr, fmt.Errorf("не удалось восстановить настройки маршрутизации: %w", saveErr))
						}
					}
				}
				dashboard.globalState = recoveredState
				if recoveredState == manager.TunnelStopped {
					dashboard.activeName = ""
					dashboard.connectedAt = time.Time{}
				}
				dashboard.lastError = startErr.Error()
			} else {
				dashboard.globalState = manager.TunnelStarted
				dashboard.activeName = name
				dashboard.connectedAt = time.Now()
			}
			_ = dashboard.Invalidate()
		})
	}()
}

func (dashboard *Dashboard) stopConnection() {
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	tunnels := make([]manager.Tunnel, len(dashboard.profiles))
	for i := range dashboard.profiles {
		tunnels[i] = dashboard.profiles[i].Tunnel
	}
	activeName := dashboard.activeName
	dashboard.globalState = manager.TunnelStopping
	_ = dashboard.Invalidate()
	if dashboard.preview {
		go func() {
			time.Sleep(450 * time.Millisecond)
			dashboard.Synchronize(func() {
				atomic.StoreUint32(&dashboard.operation, 0)
				dashboard.globalState = manager.TunnelStopped
				dashboard.activeName = ""
				dashboard.connectedAt = time.Time{}
				for i := range dashboard.profiles {
					dashboard.profiles[i].State = manager.TunnelStopped
				}
				_ = dashboard.Invalidate()
			})
		}()
		return
	}
	go func() {
		stopErr := manager.IPCClientSmartStop()
		for i := range tunnels {
			if err := tunnels[i].Stop(); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("остановка профиля %q: %w", tunnels[i].Name, err))
			} else if err := tunnels[i].WaitForStop(); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("ожидание остановки профиля %q: %w", tunnels[i].Name, err))
			}
		}
		recoveredState, stateErr := manager.IPCClientGlobalState()
		if stateErr != nil {
			recoveredState = manager.TunnelUnknown
			stopErr = errors.Join(stopErr, fmt.Errorf("проверка состояния VPN: %w", stateErr))
		} else if recoveredState != manager.TunnelStopped && recoveredState != manager.TunnelStopping {
			stopErr = errors.Join(stopErr, errors.New("менеджер не подтвердил остановку VPN"))
		}
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			dashboard.globalState = recoveredState
			if recoveredState == manager.TunnelStopped {
				dashboard.activeName = ""
				dashboard.connectedAt = time.Time{}
			} else {
				dashboard.activeName = activeName
			}
			if stopErr != nil {
				dashboard.lastError = stopErr.Error()
			}
			_ = dashboard.Invalidate()
		})
	}()
}

func (dashboard *Dashboard) saveSettings(settings smart.RoutingSettings, restartActive bool) {
	previousSettings := cloneRoutingSettings(dashboard.settings)
	normalized, err := settings.Normalized()
	if err != nil {
		dashboard.lastError = err.Error()
		_ = dashboard.Invalidate()
		return
	}
	profile := dashboard.selectedProfile()
	if profile == nil {
		return
	}
	if !dashboard.preview {
		if err := smart.SaveSettings(profile.Tunnel.Name, normalized); err != nil {
			dashboard.lastError = err.Error()
			_ = dashboard.Invalidate()
			return
		}
	}
	dashboard.settings = normalized
	dashboard.lastError = ""
	_ = dashboard.Invalidate()
	if restartActive && dashboard.globalState == manager.TunnelStarted && dashboard.activeName == profile.Tunnel.Name {
		if shouldDisconnectForRoutingSettings(normalized) {
			dashboard.lastError = "VPN отключён: в режиме «Только выбранное» не осталось сервисов или правил через VPN."
			dashboard.stopConnection()
			return
		}
		dashboard.startSelectedProfileWithRollback(&previousSettings)
	}
}

func (dashboard *Dashboard) setMode(mode smart.Mode) {
	if dashboard.operationBusy() || dashboard.selectedProfile() == nil {
		return
	}
	mode = smart.NormalizeMode(string(mode))
	if mode == smart.NormalizeMode(string(dashboard.settings.Mode)) {
		return
	}
	settings := dashboard.settings
	settings.Mode = mode
	dashboard.saveSettings(settings, true)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (dashboard *Dashboard) toggleService(id string) {
	if dashboard.operationBusy() || dashboard.selectedProfile() == nil || !serviceSelectionEnabled(dashboard.settings.Mode) {
		return
	}
	settings := dashboard.settings
	settings.SelectedApps = append([]string(nil), dashboard.settings.SelectedApps...)
	if containsString(settings.SelectedApps, id) {
		filtered := settings.SelectedApps[:0]
		for _, value := range settings.SelectedApps {
			if value != id {
				filtered = append(filtered, value)
			}
		}
		settings.SelectedApps = filtered
	} else {
		settings.SelectedApps = append(settings.SelectedApps, id)
	}
	dashboard.saveSettings(settings, true)
}

func (dashboard *Dashboard) toggleRule(index int) {
	if index < 0 || index >= len(dashboard.settings.CustomRules) || dashboard.operationBusy() {
		return
	}
	settings := dashboard.settings
	settings.CustomRules = append([]smart.CustomRule(nil), settings.CustomRules...)
	settings.CustomRules[index].Enabled = !settings.CustomRules[index].Enabled
	dashboard.saveSettings(settings, true)
}

func (dashboard *Dashboard) editRule(index int) {
	if dashboard.selectedProfile() == nil || dashboard.operationBusy() {
		return
	}
	var existing *smart.CustomRule
	if index >= 0 && index < len(dashboard.settings.CustomRules) {
		copy := dashboard.settings.CustomRules[index]
		existing = &copy
	}
	rule, ok := showRuleDialog(dashboard.Form(), existing, dashboard.settings.Mode)
	if !ok {
		return
	}
	settings := dashboard.settings
	settings.CustomRules = append([]smart.CustomRule(nil), settings.CustomRules...)
	if index >= 0 && index < len(settings.CustomRules) {
		settings.CustomRules[index] = rule
	} else {
		settings.CustomRules = append(settings.CustomRules, rule)
	}
	dashboard.saveSettings(settings, true)
}

func (dashboard *Dashboard) deleteRule(index int) {
	if index < 0 || index >= len(dashboard.settings.CustomRules) || dashboard.operationBusy() {
		return
	}
	rule := dashboard.settings.CustomRules[index]
	if walk.MsgBox(dashboard.Form(), "Удалить правило", fmt.Sprintf("Удалить правило «%s»?", rule.Name), walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	settings := dashboard.settings
	settings.CustomRules = append([]smart.CustomRule(nil), settings.CustomRules...)
	settings.CustomRules = append(settings.CustomRules[:index], settings.CustomRules[index+1:]...)
	dashboard.saveSettings(settings, true)
}

func (dashboard *Dashboard) selectProfile(index int) {
	dashboard.selectProfileWithActivation(index, true)
}

func (dashboard *Dashboard) selectProfileWithActivation(index int, switchActive bool) {
	if index < 0 || index >= len(dashboard.profiles) || dashboard.operationBusy() {
		return
	}
	wasConnected := dashboard.globalState == manager.TunnelStarted && dashboard.activeName != ""
	profile := dashboard.profiles[index]
	settings := smart.DefaultSettings()
	if !dashboard.preview {
		loadedSettings, err := smart.LoadSettings(profile.Tunnel.Name)
		if err != nil {
			dashboard.lastError = err.Error()
			_ = dashboard.Invalidate()
			if switchActive && wasConnected && dashboard.activeName != profile.Tunnel.Name {
				return
			}
		} else {
			settings = loadedSettings
		}
	}
	if switchActive && wasConnected && dashboard.activeName != profile.Tunnel.Name && shouldDisconnectForRoutingSettings(settings) {
		dashboard.lastError = "Не удалось переключить профиль: в режиме «Только выбранное» не осталось сервисов или правил через VPN."
		_ = dashboard.Invalidate()
		return
	}
	dashboard.selected = index
	dashboard.settings = settings
	if !dashboard.preview {
		_ = clientservices.SetUserKeyString("SelectedProfile", profile.Tunnel.Name)
	}
	_ = dashboard.Invalidate()
	if switchActive && wasConnected && dashboard.activeName != profile.Tunnel.Name {
		dashboard.startSelectedProfile()
	}
}

func moveProfileSettings(oldName, newName string, settings smart.RoutingSettings) error {
	if err := smart.SaveSettings(newName, settings); err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}
	return smart.DeleteSettings(oldName)
}

func (dashboard *Dashboard) editProfile(index int) {
	if dashboard.preview {
		walk.MsgBox(dashboard.Form(), "Режим предпросмотра", "Редактор профиля доступен в обычной сборке.", walk.MsgBoxIconInformation)
		return
	}
	if index < 0 || index >= len(dashboard.profiles) || dashboard.operationBusy() {
		return
	}
	profile := dashboard.profiles[index]
	config := runEditDialog(dashboard.Form(), &profile.Tunnel)
	if config == nil {
		return
	}
	originalConfig, err := profile.Tunnel.StoredConfig()
	if err != nil {
		dashboard.lastError = err.Error()
		_ = dashboard.Invalidate()
		return
	}
	oldSettings, err := smart.LoadSettings(profile.Tunnel.Name)
	if err != nil {
		dashboard.lastError = err.Error()
		_ = dashboard.Invalidate()
		return
	}
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	_ = dashboard.Invalidate()
	wasActive := dashboard.activeName == profile.Tunnel.Name && dashboard.globalState == manager.TunnelStarted
	go func() {
		var newTunnel manager.Tunnel
		operationErr := profile.Tunnel.Delete()
		deleted := operationErr == nil
		if operationErr == nil {
			operationErr = profile.Tunnel.WaitForStop()
		}
		if operationErr == nil {
			newTunnel, operationErr = manager.IPCClientNewTunnel(config)
		}
		if operationErr != nil && deleted && newTunnel.Name == "" {
			restored, restoreErr := manager.IPCClientNewTunnel(&originalConfig)
			if restoreErr != nil {
				operationErr = errors.Join(operationErr, fmt.Errorf("не удалось восстановить исходный профиль: %w", restoreErr))
			} else {
				newTunnel = restored
			}
		}
		if newTunnel.Name != "" {
			if settingsErr := moveProfileSettings(profile.Tunnel.Name, newTunnel.Name, oldSettings); settingsErr != nil {
				operationErr = errors.Join(operationErr, settingsErr)
			}
		}
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			if operationErr != nil {
				dashboard.lastError = operationErr.Error()
			}
			dashboard.loadProfiles()
			for i := range dashboard.profiles {
				if dashboard.profiles[i].Tunnel.Name == newTunnel.Name {
					dashboard.selected = i
					break
				}
			}
			if wasActive && newTunnel.Name != "" {
				dashboard.startSelectedProfile()
			}
			_ = dashboard.Invalidate()
		})
	}()
}

func (dashboard *Dashboard) deleteProfile(index int) {
	if dashboard.preview {
		walk.MsgBox(dashboard.Form(), "Режим предпросмотра", "Удаление профиля отключено в предпросмотре.", walk.MsgBoxIconInformation)
		return
	}
	if index < 0 || index >= len(dashboard.profiles) || dashboard.operationBusy() {
		return
	}
	profile := dashboard.profiles[index]
	if walk.MsgBox(dashboard.Form(), "Удалить профиль", fmt.Sprintf("Удалить VPN-профиль «%s»? Это действие нельзя отменить.", profile.Tunnel.Name), walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	_ = dashboard.Invalidate()
	go func() {
		err := profile.Tunnel.Delete()
		if err == nil {
			err = smart.DeleteSettings(profile.Tunnel.Name)
		}
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			if err != nil {
				dashboard.lastError = err.Error()
			}
			dashboard.loadProfiles()
			_ = dashboard.Invalidate()
		})
	}()
}

type unparsedProfile struct {
	Name   string
	Config string
}

const (
	maxImportedProfileSize  = 1024 * 1024
	maxImportedProfileCount = 100
)

func readProfileFiles(paths []string) ([]unparsedProfile, error) {
	profiles := make([]unparsedProfile, 0)
	var lastErr error
	for _, path := range paths {
		if len(profiles) >= maxImportedProfileCount {
			break
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".conf":
			info, err := os.Stat(path)
			if err != nil {
				lastErr = err
				continue
			}
			if info.Size() > maxImportedProfileSize {
				lastErr = fmt.Errorf("профиль %s больше 1 МБ", filepath.Base(path))
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				lastErr = err
				continue
			}
			profiles = append(profiles, unparsedProfile{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Config: string(data)})
		case ".zip":
			archive, err := zip.OpenReader(path)
			if err != nil {
				lastErr = err
				continue
			}
			for _, file := range archive.File {
				if len(profiles) >= maxImportedProfileCount {
					break
				}
				if strings.ToLower(filepath.Ext(file.Name)) != ".conf" {
					continue
				}
				if file.UncompressedSize64 > maxImportedProfileSize {
					lastErr = fmt.Errorf("профиль %s больше 1 МБ", filepath.Base(file.Name))
					continue
				}
				reader, err := file.Open()
				if err != nil {
					lastErr = err
					continue
				}
				data, readErr := io.ReadAll(io.LimitReader(reader, maxImportedProfileSize+1))
				reader.Close()
				if readErr != nil {
					lastErr = readErr
					continue
				}
				if len(data) > maxImportedProfileSize {
					lastErr = fmt.Errorf("профиль %s больше 1 МБ", filepath.Base(file.Name))
					continue
				}
				profiles = append(profiles, unparsedProfile{Name: strings.TrimSuffix(filepath.Base(file.Name), filepath.Ext(file.Name)), Config: string(data)})
			}
			archive.Close()
		}
	}
	if len(profiles) == 0 {
		if lastErr == nil {
			lastErr = errors.New("в выбранных файлах нет профилей .conf")
		}
		return nil, lastErr
	}
	return profiles, nil
}

func (dashboard *Dashboard) ImportProfiles() {
	if dashboard.preview {
		walk.MsgBox(dashboard.Form(), "Режим предпросмотра", "Импорт профиля доступен в обычной сборке.", walk.MsgBoxIconInformation)
		return
	}
	dialog := walk.FileDialog{
		Filter: "Профили AmneziaWG (*.conf, *.zip)|*.conf;*.zip|Все файлы (*.*)|*.*",
		Title:  "Импортировать VPN-профиль",
	}
	if ok, _ := dialog.ShowOpenMultiple(dashboard.Form()); !ok {
		return
	}
	profiles, err := readProfileFiles(dialog.FilePaths)
	if err != nil {
		showErrorCustom(dashboard.Form(), "Не удалось импортировать профиль", err.Error())
		return
	}
	if !atomic.CompareAndSwapUint32(&dashboard.operation, 0, 1) {
		return
	}
	_ = dashboard.Invalidate()
	go func() {
		created := 0
		createdNames := make([]string, 0, len(profiles))
		var lastErr error
		for _, item := range profiles {
			config, parseErr := conf.FromWgQuickWithUnknownEncoding(item.Config, item.Name)
			if parseErr != nil {
				lastErr = parseErr
				continue
			}
			tunnel, createErr := manager.IPCClientNewTunnel(config)
			if createErr != nil {
				lastErr = createErr
				continue
			}
			created++
			createdNames = append(createdNames, tunnel.Name)
		}
		dashboard.Synchronize(func() {
			atomic.StoreUint32(&dashboard.operation, 0)
			dashboard.loadProfiles()
			if len(createdNames) > 0 && dashboard.globalState != manager.TunnelStarted {
				for i := range dashboard.profiles {
					if dashboard.profiles[i].Tunnel.Name == createdNames[0] {
						dashboard.selectProfile(i)
						break
					}
				}
			}
			if created == 0 && lastErr != nil {
				showErrorCustom(dashboard.Form(), "Не удалось импортировать профиль", lastErr.Error())
			} else if created > 1 || lastErr != nil {
				message := fmt.Sprintf("Импортировано профилей: %d", created)
				if lastErr != nil {
					message += "\n\nПоследняя ошибка: " + lastErr.Error()
				}
				walk.MsgBox(dashboard.Form(), "Импорт профилей", message, walk.MsgBoxIconInformation)
			}
			_ = dashboard.Invalidate()
		})
	}()
}

func (dashboard *Dashboard) openRuntimeFolder() {
	path := smart.WorkDir()
	_ = os.MkdirAll(path, 0700)
	_ = exec.Command("explorer.exe", path).Start()
}

func (dashboard *Dashboard) copyDiagnostics() {
	profileName := "нет"
	endpoint := "нет"
	if profile := dashboard.selectedProfile(); profile != nil {
		profileName = profile.Tunnel.Name
		endpoint = profile.Endpoint
	}
	engine, engineErr := smart.FindEnginePath()
	if engineErr != nil {
		engine = engineErr.Error()
	}
	text := fmt.Sprintf("Pinus Smart AWG\r\nProfile: %s\r\nEndpoint: %s\r\nRouting mode: %s\r\nEngine: %s\r\nState: %d\r\nLast error: %s", profileName, endpoint, dashboard.settings.Mode, engine, dashboard.globalState, dashboard.lastError)
	if err := walk.Clipboard().SetText(text); err != nil {
		dashboard.lastError = err.Error()
	} else {
		walk.MsgBox(dashboard.Form(), "Диагностика", "Информация скопирована. Приватных ключей в ней нет.", walk.MsgBoxIconInformation)
	}
	_ = dashboard.Invalidate()
}
