/* SPDX-License-Identifier: MIT */

package manager

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"golang.org/x/sys/windows"

	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

var nativeLifecycleLock sync.Mutex

type nativeSwitchPlan struct {
	stop    []string
	wait    []string
	install bool
}

func planNativeSwitch(target string, states map[string]TunnelState) nativeSwitchPlan {
	plan := nativeSwitchPlan{install: true}
	for name, state := range states {
		if name == target {
			switch state {
			case TunnelStarted, TunnelStarting, TunnelUnknown:
				plan.install = false
			case TunnelStopping:
				plan.wait = append(plan.wait, name)
			}
			continue
		}
		if state == TunnelStopped {
			continue
		}
		plan.stop = append(plan.stop, name)
		plan.wait = append(plan.wait, name)
	}
	sort.Strings(plan.stop)
	sort.Strings(plan.wait)
	return plan
}

func executeNativeSwitch(plan nativeSwitchPlan, stop, wait func(string) error, install func() error) error {
	var switchErr error
	for _, name := range plan.stop {
		if err := stop(name); err != nil {
			switchErr = errors.Join(switchErr, fmt.Errorf("stop native tunnel %q: %w", name, err))
		}
	}
	for _, name := range plan.wait {
		if err := wait(name); err != nil {
			switchErr = errors.Join(switchErr, fmt.Errorf("wait for native tunnel %q: %w", name, err))
		}
	}
	if switchErr != nil || !plan.install {
		return switchErr
	}
	return install()
}

func normalizeNativeUninstallError(err error) error {
	if err == windows.ERROR_SERVICE_MARKED_FOR_DELETE || err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
		return nil
	}
	return err
}

func (s *ManagerService) nativeTunnelStates() (map[string]TunnelState, error) {
	names, err := conf.ListConfigNames()
	if err != nil {
		return nil, err
	}
	nameSet := make(map[string]bool, len(names))
	for _, name := range names {
		nameSet[name] = true
	}
	trackedTunnelsLock.Lock()
	for name := range trackedTunnels {
		nameSet[name] = true
	}
	trackedTunnelsLock.Unlock()

	states := make(map[string]TunnelState, len(nameSet))
	for name := range nameSet {
		state, stateErr := s.State(name)
		if stateErr != nil {
			return nil, fmt.Errorf("query native tunnel %q: %w", name, stateErr)
		}
		states[name] = state
	}
	return states, nil
}

func (s *ManagerService) stopNativeTunnelsLocked(names []string) error {
	unique := make(map[string]bool, len(names))
	ordered := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" || unique[name] {
			continue
		}
		unique[name] = true
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	var stopErr error
	for _, name := range ordered {
		if err := normalizeNativeUninstallError(UninstallTunnel(name)); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("stop native tunnel %q: %w", name, err))
		}
	}
	for _, name := range ordered {
		if err := s.WaitForStop(name); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("wait for native tunnel %q: %w", name, err))
		}
	}
	return stopErr
}

func (s *ManagerService) stopNativeTunnel(tunnelName string) error {
	nativeLifecycleLock.Lock()
	defer nativeLifecycleLock.Unlock()
	return s.stopNativeTunnelsLocked([]string{tunnelName})
}

func (s *ManagerService) stopAllNativeTunnels() error {
	nativeLifecycleLock.Lock()
	defer nativeLifecycleLock.Unlock()
	states, err := s.nativeTunnelStates()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(states))
	for name, state := range states {
		if state != TunnelStopped {
			names = append(names, name)
		}
	}
	return s.stopNativeTunnelsLocked(names)
}
