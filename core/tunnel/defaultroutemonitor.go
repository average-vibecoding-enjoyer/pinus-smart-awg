/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2021 WireGuard LLC. All Rights Reserved.
 */

package tunnel

import (
	"log"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"github.com/amnezia-vpn/amneziawg-windows/tunnel/winipcfg"
)

func bindSocketRoute(family winipcfg.AddressFamily, binder conn.BindSocketToInterface, ourLUID winipcfg.LUID, lastLUID *winipcfg.LUID, lastIndex *uint32, blackholeWhenLoop bool) error {
	r, err := winipcfg.GetIPForwardTable2(family)
	if err != nil {
		return err
	}
	lowestMetric := ^uint32(0)
	index := uint32(0)       // Zero is "unspecified", which for IP_UNICAST_IF resets the value, which is what we want.
	luid := winipcfg.LUID(0) // Hopefully luid zero is unspecified, but hard to find docs saying so.
	for i := range r {
		if r[i].DestinationPrefix.PrefixLength != 0 || r[i].InterfaceLUID == ourLUID {
			continue
		}
		ifrow, err := r[i].InterfaceLUID.Interface()
		if err != nil || ifrow.OperStatus != winipcfg.IfOperStatusUp {
			continue
		}

		iface, err := r[i].InterfaceLUID.IPInterface(family)
		if err != nil {
			continue
		}

		if r[i].Metric+iface.Metric < lowestMetric {
			lowestMetric = r[i].Metric + iface.Metric
			index = r[i].InterfaceIndex
			luid = r[i].InterfaceLUID
		}
	}
	if luid == *lastLUID && index == *lastIndex {
		return nil
	}
	blackhole := blackholeWhenLoop && index == 0
	if family == windows.AF_INET {
		log.Printf("Binding v4 socket to interface %d (blackhole=%v)", index, blackhole)
		err = binder.BindSocketToInterface4(index, blackhole)
	} else if family == windows.AF_INET6 {
		log.Printf("Binding v6 socket to interface %d (blackhole=%v)", index, blackhole)
		err = binder.BindSocketToInterface6(index, blackhole)
	}
	if err != nil {
		return err
	}
	*lastLUID, *lastIndex = luid, index
	return nil
}

func monitorDefaultRoutes(family winipcfg.AddressFamily, binder conn.BindSocketToInterface, autoMTU bool, blackholeWhenLoop bool, tun *tun.NativeTun) ([]winipcfg.ChangeCallback, error) {
	var minMTU uint32
	if family == windows.AF_INET {
		minMTU = 576
	} else if family == windows.AF_INET6 {
		minMTU = 1280
	}
	ourLUID := winipcfg.LUID(tun.LUID())
	lastLUID := winipcfg.LUID(0)
	lastIndex := ^uint32(0)
	lastMTU := uint32(0)
	doIt := func() error {
		err := bindSocketRoute(family, binder, ourLUID, &lastLUID, &lastIndex, blackholeWhenLoop)
		if err != nil {
			return err
		}
		if !autoMTU {
			return nil
		}
		mtu := uint32(0)
		if lastLUID != 0 {
			iface, err := lastLUID.Interface()
			if err != nil {
				return err
			}
			if iface.MTU > 0 {
				mtu = iface.MTU
			}
		}
		if mtu > 0 && lastMTU != mtu {
			iface, err := ourLUID.IPInterface(family)
			if err != nil {
				return err
			}
			iface.NLMTU = mtu - 80
			if iface.NLMTU < minMTU {
				iface.NLMTU = minMTU
			}
			err = iface.Set()
			if err != nil {
				return err
			}
			tun.ForceMTU(int(iface.NLMTU)) // TODO: having one MTU for both v4 and v6 kind of breaks the windows model, so right now this just gets the second one which is... bad.
			lastMTU = mtu
		}
		return nil
	}
	err := doIt()
	if err != nil {
		return nil, err
	}

	firstBurst := time.Time{}
	burstMutex := sync.Mutex{}
	closed := false
	attempts := 0
	var burstTimer *time.Timer
	runAndRetry := func() {
		if err := doIt(); err != nil {
			log.Printf("Default route binding failed (family %d): %v", family, err)
			if attempts < 5 {
				attempts++
			}
			burstTimer.Reset(time.Second * time.Duration(1<<attempts))
		} else {
			attempts = 0
		}
	}
	burstTimer = time.AfterFunc(200*time.Hour, func() {
		burstMutex.Lock()
		defer burstMutex.Unlock()
		if closed {
			return
		}
		firstBurst = time.Time{}
		runAndRetry()
	})
	burstTimer.Stop()
	cleanup := &routeMonitorCleanup{close: func() { burstMutex.Lock(); closed = true; burstTimer.Stop(); burstMutex.Unlock() }}
	bump := func() {
		burstMutex.Lock()
		defer burstMutex.Unlock()
		if closed {
			return
		}
		burstTimer.Reset(150 * time.Millisecond)
		if firstBurst.IsZero() {
			firstBurst = time.Now()
		} else if time.Since(firstBurst) > 2*time.Second {
			firstBurst = time.Time{}
			burstTimer.Stop()
			runAndRetry()
		}
	}

	cbr, err := winipcfg.RegisterRouteChangeCallback(func(notificationType winipcfg.MibNotificationType, route *winipcfg.MibIPforwardRow2) {
		if route != nil && route.DestinationPrefix.PrefixLength == 0 {
			bump()
		}
	})
	if err != nil {
		cleanup.Unregister()
		return nil, err
	}
	cbi, err := winipcfg.RegisterInterfaceChangeCallback(func(notificationType winipcfg.MibNotificationType, iface *winipcfg.MibIPInterfaceRow) {
		if notificationType == winipcfg.MibParameterNotification {
			bump()
		}
	})
	if err != nil {
		cleanup.Unregister()
		cbr.Unregister()
		return nil, err
	}
	return []winipcfg.ChangeCallback{cleanup, cbr, cbi}, nil
}

type routeMonitorCleanup struct {
	once  sync.Once
	close func()
}

func (cleanup *routeMonitorCleanup) Unregister() error { cleanup.once.Do(cleanup.close); return nil }
