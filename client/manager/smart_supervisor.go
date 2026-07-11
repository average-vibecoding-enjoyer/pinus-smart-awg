/* SPDX-License-Identifier: MIT */

package manager

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
)

var errSmartNetworkUnavailable = errors.New("no usable physical network is available")

type smartRecoveryRequest struct {
	reason string
	force  bool
	delay  time.Duration
}

type smartSupervisorConfig struct {
	pollInterval   time.Duration
	networkSettle  time.Duration
	unhealthyPolls int
	retryDelay     func(attempt int) time.Duration
}

type smartSupervisorDependencies struct {
	recover func(force bool, reason string) error
	network func() (signature string, online bool, err error)
	desired func() (tunnelName string, exists bool, err error)
	healthy func(tunnelName string) bool
}

type smartRecoverySupervisor struct {
	config   smartSupervisorConfig
	deps     smartSupervisorDependencies
	requests chan smartRecoveryRequest
	reset    chan struct{}
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

var (
	smartSupervisorLock  sync.Mutex
	smartSupervisor      *smartRecoverySupervisor
	smartSystemSuspended atomic.Bool
)

func smartRecoveryDelay(attempt int) time.Duration {
	delays := [...]time.Duration{
		time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
		time.Minute,
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt]
}

func newSmartRecoverySupervisor(config smartSupervisorConfig, deps smartSupervisorDependencies) *smartRecoverySupervisor {
	if config.pollInterval <= 0 {
		config.pollInterval = 5 * time.Second
	}
	if config.networkSettle <= 0 {
		config.networkSettle = 2500 * time.Millisecond
	}
	if config.unhealthyPolls < 1 {
		config.unhealthyPolls = 2
	}
	if config.retryDelay == nil {
		config.retryDelay = smartRecoveryDelay
	}
	return &smartRecoverySupervisor{
		config:   config,
		deps:     deps,
		requests: make(chan smartRecoveryRequest, 8),
		reset:    make(chan struct{}, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (supervisor *smartRecoverySupervisor) start() {
	go supervisor.run()
}

func (supervisor *smartRecoverySupervisor) close() {
	supervisor.stopOnce.Do(func() { close(supervisor.stop) })
	<-supervisor.done
}

func (supervisor *smartRecoverySupervisor) request(request smartRecoveryRequest) {
	if request.delay < 0 {
		request.delay = 0
	}
	select {
	case supervisor.requests <- request:
	default:
		// A full queue already guarantees a prompt recovery pass. The periodic
		// health check will schedule another one if the newest event still matters.
	}
}

func (supervisor *smartRecoverySupervisor) markHealthy() {
	select {
	case supervisor.reset <- struct{}{}:
	default:
	}
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (supervisor *smartRecoverySupervisor) run() {
	defer close(supervisor.done)
	ticker := time.NewTicker(supervisor.config.pollInterval)
	defer ticker.Stop()

	lastSignature, lastOnline, signatureErr := supervisor.deps.network()
	haveSignature := signatureErr == nil
	unhealthyCount := 0
	attempts := 0

	var timer *time.Timer
	var timerChannel <-chan time.Time
	var pending smartRecoveryRequest
	var pendingAt time.Time
	hasPending := false

	cancelPending := func() {
		stopTimer(timer)
		timer = nil
		timerChannel = nil
		hasPending = false
		pending = smartRecoveryRequest{}
		pendingAt = time.Time{}
	}

	schedule := func(request smartRecoveryRequest) {
		due := time.Now().Add(request.delay)
		if hasPending {
			pending.force = pending.force || request.force
			if pending.reason == "" {
				pending.reason = request.reason
			}
			if !due.Before(pendingAt) {
				return
			}
			stopTimer(timer)
		} else {
			pending = request
			hasPending = true
		}
		pendingAt = due
		delay := time.Until(due)
		if delay < 0 {
			delay = 0
		}
		timer = time.NewTimer(delay)
		timerChannel = timer.C
	}

	for {
		select {
		case request := <-supervisor.requests:
			schedule(request)
		case <-supervisor.reset:
			attempts = 0
			unhealthyCount = 0
			cancelPending()
		case <-ticker.C:
			signature, online, err := supervisor.deps.network()
			if err == nil {
				if haveSignature && online && (!lastOnline || signature != lastSignature) {
					schedule(smartRecoveryRequest{reason: "network changed", force: true, delay: supervisor.config.networkSettle})
				}
				lastSignature = signature
				lastOnline = online
				haveSignature = true
			}

			tunnelName, exists, desiredErr := supervisor.deps.desired()
			if desiredErr != nil {
				log.Printf("smart supervisor cannot read desired state: %v", desiredErr)
				continue
			}
			if !exists {
				unhealthyCount = 0
				attempts = 0
				continue
			}
			if online && supervisor.deps.healthy(tunnelName) {
				unhealthyCount = 0
				attempts = 0
				continue
			}
			if !online {
				unhealthyCount = 0
				continue
			}
			unhealthyCount++
			if unhealthyCount >= supervisor.config.unhealthyPolls {
				schedule(smartRecoveryRequest{reason: "health check failed"})
				unhealthyCount = 0
			}
		case <-timerChannel:
			request := pending
			timer = nil
			timerChannel = nil
			hasPending = false
			if err := supervisor.deps.recover(request.force, request.reason); err != nil {
				delay := supervisor.config.retryDelay(attempts)
				attempts++
				log.Printf("smart recovery (%s) failed; retrying in %s: %v", request.reason, delay, err)
				schedule(smartRecoveryRequest{reason: request.reason, force: request.force, delay: delay})
			} else {
				attempts = 0
				unhealthyCount = 0
			}
		case <-supervisor.stop:
			cancelPending()
			return
		}
	}
}

type smartNetworkRecord struct {
	Index     int
	Name      string
	MTU       int
	Hardware  string
	Addresses []string
}

func normalizeSmartNetworkAddress(value string) string {
	ip, network, err := net.ParseCIDR(value)
	if err != nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return ""
	}
	ones, _ := network.Mask.Size()
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String() + "/" + strconv.Itoa(ones)
	}
	return ip.Mask(network.Mask).String() + "/" + strconv.Itoa(ones)
}

func smartNetworkFingerprintFromRecords(records []smartNetworkRecord) string {
	normalized := append([]smartNetworkRecord(nil), records...)
	for index := range normalized {
		normalized[index].Addresses = append([]string(nil), normalized[index].Addresses...)
		sort.Strings(normalized[index].Addresses)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Index != normalized[j].Index {
			return normalized[i].Index < normalized[j].Index
		}
		return normalized[i].Name < normalized[j].Name
	})
	digest := sha256.New()
	for _, record := range normalized {
		_, _ = fmt.Fprintf(digest, "%d\x00%s\x00%d\x00%s\x00%s\n", record.Index, record.Name, record.MTU, record.Hardware, strings.Join(record.Addresses, ","))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func physicalSmartNetwork() (string, bool, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", false, err
	}
	records := make([]smartNetworkRecord, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || strings.EqualFold(iface.Name, smart.TunInterfaceName) {
			continue
		}
		addresses, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		normalized := make([]string, 0, len(addresses))
		for _, address := range addresses {
			if value := normalizeSmartNetworkAddress(address.String()); value != "" {
				normalized = append(normalized, value)
			}
		}
		if len(normalized) == 0 {
			continue
		}
		records = append(records, smartNetworkRecord{
			Index:     iface.Index,
			Name:      iface.Name,
			MTU:       iface.MTU,
			Hardware:  iface.HardwareAddr.String(),
			Addresses: normalized,
		})
	}
	if len(records) == 0 {
		return "", false, nil
	}
	return smartNetworkFingerprintFromRecords(records), true, nil
}

func recoverSmartDesired(force bool, reason string) error {
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	if smartSystemSuspended.Load() {
		return errors.New("system is suspended")
	}
	state, exists, err := loadSmartDesired()
	if err != nil || !exists {
		return err
	}
	_, online, err := physicalSmartNetwork()
	if err != nil {
		return err
	}
	if !online {
		return errSmartNetworkUnavailable
	}
	if !force && smartProcessHealthyFor(state.TunnelName) {
		return nil
	}
	log.Printf("[%s] recovering smart routing after %s", state.TunnelName, reason)
	service := &ManagerService{}
	return service.smartStartLocked(state.TunnelName, state.Settings, false)
}

func startSmartRecoverySupervisor() {
	smartSupervisorLock.Lock()
	defer smartSupervisorLock.Unlock()
	if smartSupervisor != nil {
		return
	}
	supervisor := newSmartRecoverySupervisor(smartSupervisorConfig{}, smartSupervisorDependencies{
		recover: recoverSmartDesired,
		network: physicalSmartNetwork,
		desired: func() (string, bool, error) {
			state, exists, err := loadSmartDesired()
			return state.TunnelName, exists, err
		},
		healthy: smartProcessHealthyFor,
	})
	smartSupervisor = supervisor
	supervisor.start()
}

func stopSmartRecoverySupervisor() {
	smartSupervisorLock.Lock()
	supervisor := smartSupervisor
	smartSupervisor = nil
	smartSupervisorLock.Unlock()
	if supervisor != nil {
		supervisor.close()
	}
}

func requestSmartRecovery(reason string, force bool, delay time.Duration) {
	smartSupervisorLock.Lock()
	supervisor := smartSupervisor
	smartSupervisorLock.Unlock()
	if supervisor != nil {
		supervisor.request(smartRecoveryRequest{reason: reason, force: force, delay: delay})
	}
}

func markSmartRecoveryHealthy() {
	smartSupervisorLock.Lock()
	supervisor := smartSupervisor
	smartSupervisorLock.Unlock()
	if supervisor != nil {
		supervisor.markHealthy()
	}
}

func pauseSmartForSuspend() error {
	smartSystemSuspended.Store(true)
	markSmartRecoveryHealthy()
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	_, exists, err := loadSmartDesired()
	if err != nil || !exists {
		return err
	}
	return smartStopUnlocked()
}

func resumeSmartAfterSuspend() {
	resumeSmartAfterSuspendWith(requestSmartRecovery)
}

func resumeSmartAfterSuspendWith(request func(reason string, force bool, delay time.Duration)) {
	smartSystemSuspended.Store(false)
	request("system resumed", true, 2*time.Second)
}
