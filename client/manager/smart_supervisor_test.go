/* SPDX-License-Identifier: MIT */

package manager

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSmartRecoveryDelayIsBounded(t *testing.T) {
	want := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute, time.Minute}
	for attempt, expected := range want {
		if got := smartRecoveryDelay(attempt); got != expected {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, got, expected)
		}
	}
}

func TestSmartNetworkFingerprintIsOrderIndependent(t *testing.T) {
	records := []smartNetworkRecord{
		{Index: 9, Name: "Wi-Fi", MTU: 1500, Hardware: "aa", Addresses: []string{"2001:db8::/64", "192.168.1.5/24"}},
		{Index: 4, Name: "Ethernet", MTU: 1500, Hardware: "bb", Addresses: []string{"10.0.0.2/24"}},
	}
	first := smartNetworkFingerprintFromRecords(records)
	records[0], records[1] = records[1], records[0]
	records[1].Addresses[0], records[1].Addresses[1] = records[1].Addresses[1], records[1].Addresses[0]
	second := smartNetworkFingerprintFromRecords(records)
	if first != second {
		t.Fatalf("network fingerprint changed with ordering: %s != %s", first, second)
	}
	records[0].Addresses[0] = "10.0.1.2/24"
	if changed := smartNetworkFingerprintFromRecords(records); changed == first {
		t.Fatal("network fingerprint ignored an IPv4 address change")
	}
}

func TestNormalizeSmartNetworkAddress(t *testing.T) {
	cases := map[string]string{
		"192.168.1.25/24":   "192.168.1.25/24",
		"2001:db8::1234/64": "2001:db8::/64",
		"fe80::1/64":        "",
		"127.0.0.1/8":       "",
	}
	for input, expected := range cases {
		if got := normalizeSmartNetworkAddress(input); got != expected {
			t.Fatalf("normalize %q = %q, want %q", input, got, expected)
		}
	}
}

func TestSmartSupervisorRetriesAfterInjectedFailure(t *testing.T) {
	var lock sync.Mutex
	calls := 0
	recovered := make(chan struct{}, 1)
	supervisor := newSmartRecoverySupervisor(smartSupervisorConfig{
		pollInterval:   50 * time.Millisecond,
		networkSettle:  5 * time.Millisecond,
		unhealthyPolls: 2,
		retryDelay:     func(int) time.Duration { return 5 * time.Millisecond },
	}, smartSupervisorDependencies{
		recover: func(bool, string) error {
			lock.Lock()
			defer lock.Unlock()
			calls++
			if calls == 1 {
				return errors.New("injected engine crash")
			}
			select {
			case recovered <- struct{}{}:
			default:
			}
			return nil
		},
		network: func() (string, bool, error) { return "network-a", true, nil },
		desired: func() (string, bool, error) { return "Phone", true, nil },
		healthy: func(string) bool { return true },
	})
	supervisor.start()
	defer supervisor.close()
	supervisor.request(smartRecoveryRequest{reason: "fault injection"})
	select {
	case <-recovered:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("supervisor did not retry after an injected recovery failure")
	}
	lock.Lock()
	defer lock.Unlock()
	if calls != 2 {
		t.Fatalf("recovery calls = %d, want 2", calls)
	}
}

func TestSmartSupervisorRecoversAfterNetworkChange(t *testing.T) {
	var lock sync.Mutex
	var initializedOnce sync.Once
	initialized := make(chan struct{})
	signature := "network-a"
	forced := make(chan bool, 1)
	supervisor := newSmartRecoverySupervisor(smartSupervisorConfig{
		pollInterval:   5 * time.Millisecond,
		networkSettle:  5 * time.Millisecond,
		unhealthyPolls: 2,
	}, smartSupervisorDependencies{
		recover: func(force bool, _ string) error {
			forced <- force
			return nil
		},
		network: func() (string, bool, error) {
			lock.Lock()
			defer lock.Unlock()
			initializedOnce.Do(func() { close(initialized) })
			return signature, true, nil
		},
		desired: func() (string, bool, error) { return "Phone", true, nil },
		healthy: func(string) bool { return true },
	})
	supervisor.start()
	defer supervisor.close()
	select {
	case <-initialized:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("supervisor did not initialize its network snapshot")
	}
	lock.Lock()
	signature = "network-b"
	lock.Unlock()
	select {
	case wasForced := <-forced:
		if !wasForced {
			t.Fatal("network change did not force route rebinding")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("network change did not trigger recovery")
	}
}

func TestSmartSupervisorRecoversAfterConsecutiveHealthFailures(t *testing.T) {
	recovered := make(chan bool, 1)
	supervisor := newSmartRecoverySupervisor(smartSupervisorConfig{
		pollInterval:   5 * time.Millisecond,
		networkSettle:  5 * time.Millisecond,
		unhealthyPolls: 2,
		retryDelay:     func(int) time.Duration { return 5 * time.Millisecond },
	}, smartSupervisorDependencies{
		recover: func(force bool, _ string) error {
			recovered <- force
			return nil
		},
		network: func() (string, bool, error) { return "network-a", true, nil },
		desired: func() (string, bool, error) { return "Phone", true, nil },
		healthy: func(string) bool { return false },
	})
	supervisor.start()
	defer supervisor.close()
	select {
	case forced := <-recovered:
		if forced {
			t.Fatal("ordinary health recovery unexpectedly forced a healthy restart")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("consecutive health failures did not trigger recovery")
	}
}

func TestSmartSupervisorHealthySignalCancelsPendingRecovery(t *testing.T) {
	recovered := make(chan struct{}, 1)
	supervisor := newSmartRecoverySupervisor(smartSupervisorConfig{
		pollInterval:   time.Second,
		networkSettle:  5 * time.Millisecond,
		unhealthyPolls: 2,
	}, smartSupervisorDependencies{
		recover: func(bool, string) error {
			recovered <- struct{}{}
			return nil
		},
		network: func() (string, bool, error) { return "network-a", true, nil },
		desired: func() (string, bool, error) { return "Phone", true, nil },
		healthy: func(string) bool { return true },
	})
	supervisor.start()
	defer supervisor.close()
	supervisor.request(smartRecoveryRequest{reason: "stale event", delay: 80 * time.Millisecond})
	time.Sleep(10 * time.Millisecond)
	supervisor.markHealthy()
	select {
	case <-recovered:
		t.Fatal("healthy signal did not cancel stale recovery")
	case <-time.After(150 * time.Millisecond):
	}
}
