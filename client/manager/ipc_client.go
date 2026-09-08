/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2022 WireGuard LLC. All Rights Reserved.
 */

package manager

import (
	"encoding/gob"
	"errors"
	"os"
	"sync"

	"github.com/amnezia-vpn/amneziawg-windows-client/updater"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

type Tunnel struct {
	Name string
}

type TunnelState int

const (
	TunnelUnknown TunnelState = iota
	TunnelStarted
	TunnelStopped
	TunnelStarting
	TunnelStopping
)

type NotificationType int

const (
	TunnelChangeNotificationType NotificationType = iota
	TunnelsChangeNotificationType
	ManagerStoppingNotificationType
	UpdateFoundNotificationType
	UpdateProgressNotificationType
)

type MethodType int

const (
	StoredConfigMethodType MethodType = iota
	RuntimeConfigMethodType
	StartMethodType
	StopMethodType
	WaitForStopMethodType
	DeleteMethodType
	StateMethodType
	GlobalStateMethodType
	CreateMethodType
	TunnelsMethodType
	QuitMethodType
	UpdateStateMethodType
	UpdateMethodType
	SmartStartMethodType
	SmartStopMethodType
	ConnectionSnapshotMethodType
	ProbeConnectionMethodType
)

var (
	rpcEncoder *gob.Encoder
	rpcDecoder *gob.Decoder
	rpcMutex   sync.Mutex
)

type TunnelChangeCallback struct {
	cb func(tunnel *Tunnel, state, globalState TunnelState, err error)
}

var callbackMutex sync.RWMutex

func snapshotCallbacks[T comparable](callbacks map[T]bool) []T {
	callbackMutex.RLock()
	defer callbackMutex.RUnlock()
	result := make([]T, 0, len(callbacks))
	for cb := range callbacks {
		result = append(result, cb)
	}
	return result
}

var tunnelChangeCallbacks = make(map[*TunnelChangeCallback]bool)

type TunnelsChangeCallback struct {
	cb func()
}

var tunnelsChangeCallbacks = make(map[*TunnelsChangeCallback]bool)

type ManagerStoppingCallback struct {
	cb func()
}

var managerStoppingCallbacks = make(map[*ManagerStoppingCallback]bool)

type UpdateFoundCallback struct {
	cb func(updateState UpdateState)
}

var updateFoundCallbacks = make(map[*UpdateFoundCallback]bool)

type UpdateProgressCallback struct {
	cb func(dp updater.DownloadProgress)
}

var updateProgressCallbacks = make(map[*UpdateProgressCallback]bool)

func InitializeIPCClient(reader, writer, events *os.File) {
	rpcDecoder = gob.NewDecoder(reader)
	rpcEncoder = gob.NewEncoder(writer)
	go func() {
		decoder := gob.NewDecoder(events)
		for {
			var notificationType NotificationType
			err := decoder.Decode(&notificationType)
			if err != nil {
				return
			}
			switch notificationType {
			case TunnelChangeNotificationType:
				var tunnel string
				err := decoder.Decode(&tunnel)
				if err != nil || len(tunnel) == 0 {
					continue
				}
				var state TunnelState
				err = decoder.Decode(&state)
				if err != nil {
					continue
				}
				var globalState TunnelState
				err = decoder.Decode(&globalState)
				if err != nil {
					continue
				}
				var errStr string
				err = decoder.Decode(&errStr)
				if err != nil {
					continue
				}
				var retErr error
				if len(errStr) > 0 {
					retErr = errors.New(errStr)
				}
				if state == TunnelUnknown {
					continue
				}
				t := &Tunnel{tunnel}
				for _, cb := range snapshotCallbacks(tunnelChangeCallbacks) {
					cb.cb(t, state, globalState, retErr)
				}
			case TunnelsChangeNotificationType:
				for _, cb := range snapshotCallbacks(tunnelsChangeCallbacks) {
					cb.cb()
				}
			case ManagerStoppingNotificationType:
				for _, cb := range snapshotCallbacks(managerStoppingCallbacks) {
					cb.cb()
				}
			case UpdateFoundNotificationType:
				var state UpdateState
				err = decoder.Decode(&state)
				if err != nil {
					continue
				}
				for _, cb := range snapshotCallbacks(updateFoundCallbacks) {
					cb.cb(state)
				}
			case UpdateProgressNotificationType:
				var dp updater.DownloadProgress
				err = decoder.Decode(&dp.Activity)
				if err != nil {
					continue
				}
				err = decoder.Decode(&dp.BytesDownloaded)
				if err != nil {
					continue
				}
				err = decoder.Decode(&dp.BytesTotal)
				if err != nil {
					continue
				}
				var errStr string
				err = decoder.Decode(&errStr)
				if err != nil {
					continue
				}
				if len(errStr) > 0 {
					dp.Error = errors.New(errStr)
				}
				err = decoder.Decode(&dp.Complete)
				if err != nil {
					continue
				}
				for _, cb := range snapshotCallbacks(updateProgressCallbacks) {
					cb.cb(dp)
				}
			}
		}
	}()
}

func rpcDecodeError() error {
	var str string
	err := rpcDecoder.Decode(&str)
	if err != nil {
		return err
	}
	if len(str) == 0 {
		return nil
	}
	return errors.New(str)
}

func (t *Tunnel) StoredConfig() (c conf.Config, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(StoredConfigMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(t.Name)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&c)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func (t *Tunnel) RuntimeConfig() (c conf.Config, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(RuntimeConfigMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(t.Name)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&c)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func (t *Tunnel) Start() (err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(StartMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(t.Name)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func (t *Tunnel) Stop() (err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(StopMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(t.Name)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func (t *Tunnel) Toggle() (oldState TunnelState, err error) {
	oldState, err = t.State()
	if err != nil {
		oldState = TunnelUnknown
		return
	}
	if oldState == TunnelStarted {
		err = t.Stop()
	} else if oldState == TunnelStopped {
		err = t.Start()
	}
	return
}

func (t *Tunnel) WaitForStop() (err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(WaitForStopMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(t.Name)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func (t *Tunnel) Delete() (err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(DeleteMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(t.Name)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func (t *Tunnel) State() (tunnelState TunnelState, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(StateMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(t.Name)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&tunnelState)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func IPCClientGlobalState() (tunnelState TunnelState, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(GlobalStateMethodType)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&tunnelState)
	if err != nil {
		return
	}
	return
}

func IPCClientNewTunnel(conf *conf.Config) (tunnel Tunnel, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(CreateMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(*conf)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&tunnel)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func IPCClientTunnels() (tunnels []Tunnel, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(TunnelsMethodType)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&tunnels)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func IPCClientQuit(stopTunnelsOnQuit bool) (alreadyQuit bool, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(QuitMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(stopTunnelsOnQuit)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&alreadyQuit)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func IPCClientUpdateState() (updateState UpdateState, err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(UpdateStateMethodType)
	if err != nil {
		return
	}
	err = rpcDecoder.Decode(&updateState)
	if err != nil {
		return
	}
	return
}

func IPCClientUpdate() error {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	return rpcEncoder.Encode(UpdateMethodType)
}

func IPCClientSmartStart(tunnelName, mode string) (err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(SmartStartMethodType)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(tunnelName)
	if err != nil {
		return
	}
	err = rpcEncoder.Encode(mode)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func IPCClientSmartStop() (err error) {
	rpcMutex.Lock()
	defer rpcMutex.Unlock()

	err = rpcEncoder.Encode(SmartStopMethodType)
	if err != nil {
		return
	}
	err = rpcDecodeError()
	return
}

func IPCClientRegisterTunnelChange(cb func(tunnel *Tunnel, state, globalState TunnelState, err error)) *TunnelChangeCallback {
	s := &TunnelChangeCallback{cb}
	callbackMutex.Lock()
	tunnelChangeCallbacks[s] = true
	callbackMutex.Unlock()
	return s
}

func (cb *TunnelChangeCallback) Unregister() {
	callbackMutex.Lock()
	delete(tunnelChangeCallbacks, cb)
	callbackMutex.Unlock()
}

func IPCClientRegisterTunnelsChange(cb func()) *TunnelsChangeCallback {
	s := &TunnelsChangeCallback{cb}
	callbackMutex.Lock()
	tunnelsChangeCallbacks[s] = true
	callbackMutex.Unlock()
	return s
}

func (cb *TunnelsChangeCallback) Unregister() {
	callbackMutex.Lock()
	delete(tunnelsChangeCallbacks, cb)
	callbackMutex.Unlock()
}

func IPCClientRegisterManagerStopping(cb func()) *ManagerStoppingCallback {
	s := &ManagerStoppingCallback{cb}
	callbackMutex.Lock()
	managerStoppingCallbacks[s] = true
	callbackMutex.Unlock()
	return s
}

func (cb *ManagerStoppingCallback) Unregister() {
	callbackMutex.Lock()
	delete(managerStoppingCallbacks, cb)
	callbackMutex.Unlock()
}

func IPCClientRegisterUpdateFound(cb func(updateState UpdateState)) *UpdateFoundCallback {
	s := &UpdateFoundCallback{cb}
	callbackMutex.Lock()
	updateFoundCallbacks[s] = true
	callbackMutex.Unlock()
	return s
}

func (cb *UpdateFoundCallback) Unregister() {
	callbackMutex.Lock()
	delete(updateFoundCallbacks, cb)
	callbackMutex.Unlock()
}

func IPCClientRegisterUpdateProgress(cb func(dp updater.DownloadProgress)) *UpdateProgressCallback {
	s := &UpdateProgressCallback{cb}
	callbackMutex.Lock()
	updateProgressCallbacks[s] = true
	callbackMutex.Unlock()
	return s
}

func (cb *UpdateProgressCallback) Unregister() {
	callbackMutex.Lock()
	delete(updateProgressCallbacks, cb)
	callbackMutex.Unlock()
}
