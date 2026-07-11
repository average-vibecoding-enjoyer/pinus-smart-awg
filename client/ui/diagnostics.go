/* SPDX-License-Identifier: MIT */

package ui

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/manager"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

type diagnosticLevel int

const (
	diagnosticInfo diagnosticLevel = iota
	diagnosticGood
	diagnosticWarning
	diagnosticFailure
)

type diagnosticCheck struct {
	Name   string
	Detail string
	Level  diagnosticLevel
}

type diagnosticReport struct {
	GeneratedAt time.Time
	Checks      []diagnosticCheck
}

func (report diagnosticReport) String() string {
	var output strings.Builder
	output.WriteString("Component checks: ")
	output.WriteString(report.GeneratedAt.UTC().Format(time.RFC3339))
	output.WriteString("\r\n")
	for _, check := range report.Checks {
		level := "INFO"
		switch check.Level {
		case diagnosticGood:
			level = "OK"
		case diagnosticWarning:
			level = "WARN"
		case diagnosticFailure:
			level = "FAIL"
		}
		fmt.Fprintf(&output, "%s: [%s] %s\r\n", check.Name, level, check.Detail)
	}
	return output.String()
}

func (dashboard *Dashboard) runDiagnostics() {
	if !atomic.CompareAndSwapUint32(&dashboard.diagnosticsBusy, 0, 1) {
		return
	}
	if dashboard.preview {
		go func() {
			time.Sleep(450 * time.Millisecond)
			if dashboard.presetContext.Err() != nil {
				return
			}
			report := diagnosticReport{GeneratedAt: time.Now(), Checks: []diagnosticCheck{
				{"Менеджер", "доступен за 2 ms, состояние 0", diagnosticGood},
				{"Профили", "активных: 0", diagnosticGood},
				{"Конфиг", "IPv4, IPv6 закрыт без утечки, DNS-серверов: 1", diagnosticGood},
				{"DNS endpoint", "сервер задан IP-адресом", diagnosticGood},
				{"Engine SHA", "проверен: amnezia-box.exe", diagnosticGood},
				{"Сеть", "активных интерфейсов: 2", diagnosticGood},
				{"Маршрут", "VPN сейчас отключён", diagnosticInfo},
				{"Пресеты", "rev. 2, signed update", diagnosticGood},
			}}
			dashboard.Synchronize(func() {
				dashboard.diagnostics = report
				atomic.StoreUint32(&dashboard.diagnosticsBusy, 0)
				_ = dashboard.Invalidate()
			})
		}()
		return
	}
	profile := manager.Tunnel{}
	if selected := dashboard.selectedProfile(); selected != nil {
		profile = selected.Tunnel
	}
	settings := cloneRoutingSettings(dashboard.settings)
	_ = dashboard.Invalidate()

	go func() {
		ctx, cancel := context.WithTimeout(dashboard.presetContext, 10*time.Second)
		defer cancel()
		report := collectDiagnostics(ctx, profile, settings)
		if dashboard.presetContext.Err() != nil {
			return
		}
		dashboard.Synchronize(func() {
			dashboard.diagnostics = report
			atomic.StoreUint32(&dashboard.diagnosticsBusy, 0)
			_ = dashboard.Invalidate()
		})
	}()
}

func collectDiagnostics(ctx context.Context, profile manager.Tunnel, settings smart.RoutingSettings) diagnosticReport {
	report := diagnosticReport{GeneratedAt: time.Now()}
	started := time.Now()
	globalState, managerErr := manager.IPCClientGlobalState()
	if managerErr != nil {
		report.Checks = append(report.Checks, diagnosticCheck{"Менеджер", managerErr.Error(), diagnosticFailure})
	} else {
		report.Checks = append(report.Checks, diagnosticCheck{"Менеджер", fmt.Sprintf("доступен за %s, состояние %d", time.Since(started).Round(time.Millisecond), globalState), diagnosticGood})
	}

	activeCount := 0
	tunnels, tunnelsErr := manager.IPCClientTunnels()
	if tunnelsErr == nil {
		for index := range tunnels {
			state, stateErr := tunnels[index].State()
			if stateErr == nil && (state == manager.TunnelStarted || state == manager.TunnelStarting) {
				activeCount++
			}
		}
	}
	if tunnelsErr != nil {
		report.Checks = append(report.Checks, diagnosticCheck{"Профили", tunnelsErr.Error(), diagnosticWarning})
	} else if activeCount > 1 {
		report.Checks = append(report.Checks, diagnosticCheck{"Профили", fmt.Sprintf("одновременно активны: %d", activeCount), diagnosticFailure})
	} else {
		report.Checks = append(report.Checks, diagnosticCheck{"Профили", fmt.Sprintf("активных: %d", activeCount), diagnosticGood})
	}

	var profileConfigAvailable bool
	if profile.Name == "" {
		report.Checks = append(report.Checks, diagnosticCheck{"Конфиг", "профиль не выбран", diagnosticWarning})
	} else if config, err := profile.StoredConfig(); err != nil {
		report.Checks = append(report.Checks, diagnosticCheck{"Конфиг", err.Error(), diagnosticFailure})
	} else {
		profileConfigAvailable = true
		support := smart.DetectProfileNetworkSupport(&config)
		families := networkSupportText(support)
		level := diagnosticGood
		if !support.IPv4 && !support.IPv6 {
			level = diagnosticFailure
		}
		report.Checks = append(report.Checks, diagnosticCheck{"Конфиг", fmt.Sprintf("%s, DNS-серверов: %d", families, len(config.Interface.DNS)), level})
		report.Checks = append(report.Checks, resolveEndpointCheck(ctx, &config))
	}

	if enginePath, err := smart.FindEnginePath(); err != nil {
		report.Checks = append(report.Checks, diagnosticCheck{"Engine SHA", err.Error(), diagnosticFailure})
	} else {
		report.Checks = append(report.Checks, diagnosticCheck{"Engine SHA", "проверен: " + filepath.Base(enginePath), diagnosticGood})
	}

	physicalCount := physicalInterfaceCount()
	if physicalCount == 0 {
		report.Checks = append(report.Checks, diagnosticCheck{"Сеть", "нет активного физического интерфейса", diagnosticFailure})
	} else {
		report.Checks = append(report.Checks, diagnosticCheck{"Сеть", fmt.Sprintf("активных интерфейсов: %d", physicalCount), diagnosticGood})
	}

	smartExpected := smart.NormalizeMode(string(settings.Mode)) != smart.ModeAll || smart.HasEnabledCustomRules(settings)
	if managerErr == nil && globalState == manager.TunnelStarted && smartExpected {
		if iface, err := net.InterfaceByName(smart.TunInterfaceName); err != nil || iface.Flags&net.FlagUp == 0 {
			detail := "smart TUN не найден"
			if err == nil {
				detail = "smart TUN выключен"
			}
			report.Checks = append(report.Checks, diagnosticCheck{"Маршрут", detail, diagnosticFailure})
		} else {
			report.Checks = append(report.Checks, diagnosticCheck{"Маршрут", fmt.Sprintf("smart TUN поднят, MTU %d", iface.MTU), diagnosticGood})
		}
	} else if managerErr == nil && globalState == manager.TunnelStarted {
		report.Checks = append(report.Checks, diagnosticCheck{"Маршрут", "нативный AWG full-tunnel", diagnosticGood})
	} else {
		report.Checks = append(report.Checks, diagnosticCheck{"Маршрут", "VPN сейчас отключён", diagnosticInfo})
	}

	presets := smart.ServiceCatalogStatus()
	presetLevel := diagnosticGood
	presetDetail := fmt.Sprintf("rev. %d, %s", presets.Revision, presets.Source)
	if presets.LastError != "" {
		presetLevel = diagnosticWarning
		presetDetail += ", " + presets.LastError
	}
	report.Checks = append(report.Checks, diagnosticCheck{"Пресеты", presetDetail, presetLevel})
	if !profileConfigAvailable && profile.Name != "" {
		report.Checks = append(report.Checks, diagnosticCheck{"Итог", "исправь чтение профиля перед подключением", diagnosticFailure})
	}
	return report
}

func networkSupportText(support smart.ProfileNetworkSupport) string {
	switch {
	case support.IPv4 && support.IPv6:
		return "IPv4 + IPv6"
	case support.IPv4:
		return "IPv4, IPv6 закрыт без утечки"
	case support.IPv6:
		return "IPv6, IPv4 закрыт без утечки"
	default:
		return "нет подходящего default route"
	}
}

func resolveEndpointCheck(ctx context.Context, config *conf.Config) diagnosticCheck {
	if len(config.Peers) == 0 || config.Peers[0].Endpoint.IsEmpty() {
		return diagnosticCheck{"DNS endpoint", "endpoint отсутствует", diagnosticFailure}
	}
	host := config.Peers[0].Endpoint.Host
	if net.ParseIP(host) != nil {
		return diagnosticCheck{"DNS endpoint", "сервер задан IP-адресом", diagnosticGood}
	}
	started := time.Now()
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return diagnosticCheck{"DNS endpoint", err.Error(), diagnosticFailure}
	}
	if len(addresses) == 0 {
		return diagnosticCheck{"DNS endpoint", "DNS вернул пустой ответ", diagnosticFailure}
	}
	return diagnosticCheck{"DNS endpoint", fmt.Sprintf("%d адресов за %s", len(addresses), time.Since(started).Round(time.Millisecond)), diagnosticGood}
}

func physicalInterfaceCount() int {
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0
	}
	count := 0
	for _, iface := range interfaces {
		if iface.Name == smart.TunInterfaceName || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		count++
	}
	return count
}
