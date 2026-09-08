/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2021 WireGuard LLC. All Rights Reserved.
 */

package conf

import "sync"

var storeCallbackMutex sync.RWMutex

func notifyStoreChanged() {
	storeCallbackMutex.RLock()
	callbacks := make([]*StoreCallback, 0, len(storeCallbacks))
	for cb := range storeCallbacks {
		callbacks = append(callbacks, cb)
	}
	storeCallbackMutex.RUnlock()
	for _, cb := range callbacks {
		cb.cb()
	}
}

type StoreCallback struct {
	cb func()
}

var storeCallbacks = make(map[*StoreCallback]bool)

func RegisterStoreChangeCallback(cb func()) *StoreCallback {
	startWatchingConfigDir()
	cb()
	s := &StoreCallback{cb}
	storeCallbackMutex.Lock()
	storeCallbacks[s] = true
	storeCallbackMutex.Unlock()
	return s
}

func (cb *StoreCallback) Unregister() {
	storeCallbackMutex.Lock()
	delete(storeCallbacks, cb)
	storeCallbackMutex.Unlock()
}
