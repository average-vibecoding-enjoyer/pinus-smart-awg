/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2022 WireGuard LLC. All Rights Reserved.
 */

package manager

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"github.com/amnezia-vpn/amneziawg-windows-client/updater"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
	"github.com/amnezia-vpn/amneziawg-windows/services"
	"github.com/amnezia-vpn/amneziawg-windows/tunnel/firewall"
)

var (
	managerServices     = make(map[*ManagerService]bool)
	managerServicesLock sync.RWMutex
	haveQuit            uint32
	quitManagersChan    = make(chan struct{}, 1)
)

type ManagerService struct {
	events        *os.File
	eventLock     sync.Mutex
	eventQueue    chan []byte
	eventDone     chan struct{}
	elevatedToken windows.Token
}

func (s *ManagerService) StoredConfig(tunnelName string) (*conf.Config, error) {
	conf, err := conf.LoadFromName(tunnelName)
	if err != nil {
		return nil, err
	}
	if s.elevatedToken == 0 {
		conf.Redact()
	}
	return conf, nil
}

func (s *ManagerService) RuntimeConfig(tunnelName string) (*conf.Config, error) {
	storedConfig, err := conf.LoadFromName(tunnelName)
	if err != nil {
		return nil, err
	}
	pipe, err := connectTunnelServicePipe(tunnelName)
	if err != nil {
		return nil, err
	}
	pipe.SetDeadline(time.Now().Add(time.Second * 2))
	_, err = pipe.Write([]byte("get=1\n\n"))
	if err == windows.ERROR_NO_DATA {
		log.Println("IPC pipe closed unexpectedly, so reopening")
		pipe.Unlock()
		disconnectTunnelServicePipe(tunnelName)
		pipe, err = connectTunnelServicePipe(tunnelName)
		if err != nil {
			return nil, err
		}
		pipe.SetDeadline(time.Now().Add(time.Second * 2))
		_, err = pipe.Write([]byte("get=1\n\n"))
	}
	if err != nil {
		pipe.Unlock()
		disconnectTunnelServicePipe(tunnelName)
		return nil, err
	}
	conf, err := conf.FromUAPI(pipe, storedConfig)
	pipe.Unlock()
	if err != nil {
		return nil, err
	}
	if s.elevatedToken == 0 {
		conf.Redact()
	}
	return conf, nil
}

func (s *ManagerService) Start(tunnelName string) error {
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	return s.startNativeUnlocked(tunnelName)
}
func (s *ManagerService) startNativeUnlocked(tunnelName string) error {
	if err := previewConnectionPreflight(); err != nil {
		return err
	}
	c, err := conf.LoadFromName(tunnelName)
	if err != nil {
		return err
	}
	path, err := c.Path()
	if err != nil {
		return err
	}
	if smartAnyStarted() {
		return errors.New("smart routing is active; stop it before starting a native tunnel")
	}

	nativeLifecycleLock.Lock()
	defer nativeLifecycleLock.Unlock()
	states, err := s.nativeTunnelStates()
	if err != nil {
		return err
	}
	plan := planNativeSwitch(tunnelName, states)
	switchErr := executeNativeSwitch(
		plan,
		func(name string) error { return normalizeNativeUninstallError(UninstallTunnel(name)) },
		s.WaitForStop,
		func() error { return InstallTunnel(path) },
	)
	if switchErr != nil {
		// Restore only predecessors known to have been active, never the failed
		// target. Native lifecycle ownership is still held here.
		for _, name := range plan.stop {
			if states[name] != TunnelStarted && states[name] != TunnelStarting {
				continue
			}
			state, stateErr := s.State(name)
			if stateErr != nil {
				switchErr = errors.Join(switchErr, stateErr)
				continue
			}
			if state != TunnelStopped {
				continue
			}
			prior, restoreErr := conf.LoadFromName(name)
			if restoreErr == nil {
				var priorPath string
				priorPath, restoreErr = prior.Path()
				if restoreErr == nil {
					restoreErr = InstallTunnel(priorPath)
				}
			}
			if restoreErr != nil {
				switchErr = errors.Join(switchErr, fmt.Errorf("restore %s: %w", name, restoreErr))
			}
		}
		return switchErr
	}
	if err := clearSmartDesired(); err != nil {
		rollbackErr := s.stopNativeTunnelsLocked([]string{tunnelName})
		return errors.Join(fmt.Errorf("clear smart recovery state after native start: %w", err), rollbackErr)
	}
	return nil
}

func (s *ManagerService) Stop(tunnelName string) error {
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	before, exists, _ := loadSmartDesired()
	_, wasSmart := smartStateForTunnel(tunnelName)
	clearProtection := wasSmart || (exists && before.TunnelName == tunnelName)
	desiredErr := clearSmartDesiredForTunnel(tunnelName)
	var smartErr error
	if _, matches := smartStateForTunnel(tunnelName); matches {
		smartErr = smartStopUnlocked()
	}
	nativeErr := s.stopNativeTunnel(tunnelName)
	var guardErr error
	if clearProtection {
		guardErr = firewall.ClearPreviewGuard()
	}
	return errors.Join(desiredErr, smartErr, nativeErr, guardErr)
}

func (s *ManagerService) WaitForStop(tunnelName string) error {
	serviceName, err := services.ServiceNameOfTunnel(tunnelName)
	if err != nil {
		return err
	}
	m, err := serviceManager()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		service, err := m.OpenService(serviceName)
		if err == nil {
			service.Close()
		} else if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
			return nil
		} else if err != windows.ERROR_SERVICE_MARKED_FOR_DELETE {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for tunnel %q to stop", tunnelName)
		}
		time.Sleep(time.Second / 3)
	}
}

func (s *ManagerService) Delete(tunnelName string) error {
	if s.elevatedToken == 0 {
		return windows.ERROR_ACCESS_DENIED
	}
	err := s.Stop(tunnelName)
	if err != nil {
		return err
	}
	return conf.DeleteName(tunnelName)
}

func (s *ManagerService) State(tunnelName string) (TunnelState, error) {
	if smartState, ok := smartStateForTunnel(tunnelName); ok {
		return smartState, nil
	}
	serviceName, err := services.ServiceNameOfTunnel(tunnelName)
	if err != nil {
		return 0, err
	}
	m, err := serviceManager()
	if err != nil {
		return 0, err
	}
	service, err := m.OpenService(serviceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return TunnelStopped, nil
		}
		return TunnelUnknown, err
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return TunnelUnknown, err
	}
	switch status.State {
	case svc.Stopped:
		return TunnelStopped, nil
	case svc.StopPending:
		return TunnelStopping, nil
	case svc.Running:
		return TunnelStarted, nil
	case svc.StartPending:
		return TunnelStarting, nil
	default:
		return TunnelUnknown, nil
	}
}

func (s *ManagerService) GlobalState() TunnelState {
	if smartAnyStarted() {
		return TunnelStarted
	}
	return trackedTunnelsGlobalState()
}

func (s *ManagerService) Create(tunnelConfig *conf.Config) (*Tunnel, error) {
	if s.elevatedToken == 0 {
		return nil, windows.ERROR_ACCESS_DENIED
	}
	err := tunnelConfig.Save(true)
	if err != nil {
		return nil, err
	}
	return &Tunnel{tunnelConfig.Name}, nil
	// TODO: handle already existing situation
	// TODO: handle already running and existing situation
}

func (s *ManagerService) Tunnels() ([]Tunnel, error) {
	names, err := conf.ListConfigNames()
	if err != nil {
		return nil, err
	}
	tunnels := make([]Tunnel, len(names))
	for i := 0; i < len(tunnels); i++ {
		tunnels[i].Name = names[i]
	}
	return tunnels, nil
	// TODO: account for running ones that aren't in the configuration store somehow
}

func (s *ManagerService) Quit(stopTunnelsOnQuit bool) (alreadyQuit bool, err error) {
	if s.elevatedToken == 0 {
		return false, windows.ERROR_ACCESS_DENIED
	}
	if !atomic.CompareAndSwapUint32(&haveQuit, 0, 1) {
		return true, nil
	}

	defer func() {
		if err != nil {
			atomic.StoreUint32(&haveQuit, 0)
		}
	}()
	var names []string
	if stopTunnelsOnQuit {
		names, err = conf.ListConfigNames()
		if err != nil {
			return false, err
		}
	}
	// Work around potential race condition of delivering messages to the wrong process by removing from notifications.
	managerServicesLock.Lock()
	s.eventLock.Lock()
	s.events = nil
	s.eventLock.Unlock()
	delete(managerServices, s)
	managerServicesLock.Unlock()

	if stopTunnelsOnQuit {
		_ = smartStop()
		for _, name := range names {
			UninstallTunnel(name)
		}
	}

	quitManagersChan <- struct{}{}
	return false, nil
}

func (s *ManagerService) UpdateState() UpdateState {
	return updateState
}

func (s *ManagerService) Update() {
	if s.elevatedToken == 0 {
		return
	}
	IPCServerNotifyUpdateProgress(updater.DownloadProgress{Error: errors.New("Pinus Preview обновляется отдельным проверенным пакетом")})
}

func (s *ManagerService) ServeConn(reader io.Reader, writer io.Writer) {
	decoder := gob.NewDecoder(reader)
	encoder := gob.NewEncoder(writer)
	for {
		var methodType MethodType
		err := decoder.Decode(&methodType)
		if err != nil {
			return
		}
		switch methodType {
		case StoredConfigMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			config, retErr := s.StoredConfig(tunnelName)
			if config == nil {
				config = &conf.Config{}
			}
			err = encoder.Encode(*config)
			if err != nil {
				return
			}
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case RuntimeConfigMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			config, retErr := s.RuntimeConfig(tunnelName)
			if config == nil {
				config = &conf.Config{}
			}
			err = encoder.Encode(*config)
			if err != nil {
				return
			}
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case StartMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			retErr := s.Start(tunnelName)
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case StopMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			retErr := s.Stop(tunnelName)
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case WaitForStopMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			retErr := s.WaitForStop(tunnelName)
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case DeleteMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			retErr := s.Delete(tunnelName)
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case StateMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			state, retErr := s.State(tunnelName)
			err = encoder.Encode(state)
			if err != nil {
				return
			}
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case GlobalStateMethodType:
			state := s.GlobalState()
			err = encoder.Encode(state)
			if err != nil {
				return
			}
		case CreateMethodType:
			var config conf.Config
			err := decoder.Decode(&config)
			if err != nil {
				return
			}
			tunnel, retErr := s.Create(&config)
			if tunnel == nil {
				tunnel = &Tunnel{}
			}
			err = encoder.Encode(tunnel)
			if err != nil {
				return
			}
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case TunnelsMethodType:
			tunnels, retErr := s.Tunnels()
			err = encoder.Encode(tunnels)
			if err != nil {
				return
			}
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case QuitMethodType:
			var stopTunnelsOnQuit bool
			err := decoder.Decode(&stopTunnelsOnQuit)
			if err != nil {
				return
			}
			alreadyQuit, retErr := s.Quit(stopTunnelsOnQuit)
			err = encoder.Encode(alreadyQuit)
			if err != nil {
				return
			}
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case UpdateStateMethodType:
			updateState := s.UpdateState()
			err = encoder.Encode(updateState)
			if err != nil {
				return
			}
		case UpdateMethodType:
			s.Update()
		case SmartStartMethodType:
			var tunnelName string
			err := decoder.Decode(&tunnelName)
			if err != nil {
				return
			}
			var mode string
			err = decoder.Decode(&mode)
			if err != nil {
				return
			}
			retErr := s.SmartStart(tunnelName, mode)
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		case ConnectionSnapshotMethodType:
			result, retErr := s.ConnectionSnapshot()
			if encoder.Encode(result) != nil || encoder.Encode(errToString(retErr)) != nil {
				return
			}
		case ProbeConnectionMethodType:
			result, retErr := s.ProbeConnection()
			if encoder.Encode(result) != nil || encoder.Encode(errToString(retErr)) != nil {
				return
			}
		case SmartStopMethodType:
			retErr := s.SmartStop()
			err = encoder.Encode(errToString(retErr))
			if err != nil {
				return
			}
		default:
			return
		}
	}
}

func IPCServerListen(reader, writer, events *os.File, elevatedToken windows.Token) {
	service := &ManagerService{
		events:     events,
		eventQueue: make(chan []byte, 64), eventDone: make(chan struct{}),
		elevatedToken: elevatedToken,
	}

	go func() {
		managerServicesLock.Lock()
		managerServices[service] = true
		managerServicesLock.Unlock()
		go service.pumpEvents()
		service.ServeConn(reader, writer)
		close(service.eventDone)
		managerServicesLock.Lock()
		service.eventLock.Lock()
		service.events = nil
		service.eventLock.Unlock()
		delete(managerServices, service)
		managerServicesLock.Unlock()
	}()
}

var notificationOrder sync.Mutex

func notifyAll(notificationType NotificationType, adminOnly bool, ifaces ...any) {
	notificationOrder.Lock()
	defer notificationOrder.Unlock()

	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(notificationType)
	if err != nil {
		return
	}
	for _, iface := range ifaces {
		err = encoder.Encode(iface)
		if err != nil {
			return
		}
	}

	managerServicesLock.RLock()
	for m := range managerServices {
		if m.elevatedToken == 0 && adminOnly {
			continue
		}
		queueLatestEvent(m.eventQueue, buf.Bytes())
	}
	managerServicesLock.RUnlock()
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func IPCServerNotifyTunnelChange(name string, state TunnelState, err error) {
	globalState := trackedTunnelsGlobalState()
	if smartAnyStarted() {
		globalState = TunnelStarted
	}
	notifyAll(TunnelChangeNotificationType, false, name, state, globalState, errToString(err))
}

func IPCServerNotifyTunnelsChange() {
	notifyAll(TunnelsChangeNotificationType, false)
}

func IPCServerNotifyUpdateFound(state UpdateState) {
	notifyAll(UpdateFoundNotificationType, false, state)
}

func IPCServerNotifyUpdateProgress(dp updater.DownloadProgress) {
	notifyAll(UpdateProgressNotificationType, true, dp.Activity, dp.BytesDownloaded, dp.BytesTotal, errToString(dp.Error), dp.Complete)
}

func IPCServerNotifyManagerStopping() {
	notifyAll(ManagerStoppingNotificationType, false)
	time.Sleep(time.Millisecond * 200)
}
