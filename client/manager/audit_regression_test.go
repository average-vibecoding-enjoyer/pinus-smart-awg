package manager

import (
	"errors"
	"testing"
	"time"
)

// This test invokes the production event dispatcher and resume state transition.
// It never calls pauseSmartForSuspend: the fake pause contains only the exact
// first state transition in that function, without config, process or OS access.
// Capturing the asynchronous callback represents suspension before its goroutine
// is scheduled, followed by a resume event arriving before the callback runs.
func TestAuditDelayedSuspendCannotOverwriteResume(t *testing.T) {
	oldSuspended := smartSystemSuspended.Load()
	smartSystemSuspended.Store(false)
	defer smartSystemSuspended.Store(oldSuspended)

	var queuedSuspend func()
	resumeRequests := 0
	runAsync := func(operation func()) { queuedSuspend = operation }
	pause := func() error {
		smartSystemSuspended.Store(true)
		return nil
	}
	resume := func() {
		resumeSmartAfterSuspendWith(func(string, bool, time.Duration) {
			resumeRequests++
		})
	}

	if !handleSmartPowerEvent(pbtAPMSuspend, runAsync, pause, resume) || queuedSuspend == nil {
		t.Fatal("suspend event was not queued")
	}
	if !handleSmartPowerEvent(pbtAPMResumeAuto, runAsync, pause, resume) {
		t.Fatal("resume event was not handled")
	}
	if smartSystemSuspended.Load() || resumeRequests != 1 {
		t.Fatal("resume did not clear suspended state and request recovery")
	}
	queuedSuspend()
	if smartSystemSuspended.Load() {
		t.Fatal("a delayed pre-resume suspend callback restored suspended=true after resume; production recovery now rejects all attempts as system is suspended")
	}
}

// All four supervisor dependencies are fakes. The only activity is a goroutine,
// channels and short timers. No physicalSmartNetwork, desired file, engine,
// Windows service, process start/stop, interface or system setting is touched.
func TestAuditHealthPollingHonorsRecoveryBackoff(t *testing.T) {
	calls := make(chan time.Time, 16)
	supervisor := newSmartRecoverySupervisor(smartSupervisorConfig{
		pollInterval:   10 * time.Millisecond,
		networkSettle:  5 * time.Millisecond,
		unhealthyPolls: 1,
		retryDelay:     func(int) time.Duration { return time.Hour },
	}, smartSupervisorDependencies{
		recover: func(bool, string) error {
			select {
			case calls <- time.Now():
			default:
			}
			return errors.New("injected permanent failure")
		},
		network: func() (string, bool, error) { return "stable-network", true, nil },
		desired: func() (string, bool, error) { return "synthetic-profile", true, nil },
		healthy: func(string) bool { return false },
	})
	supervisor.start()
	defer supervisor.close()
	supervisor.request(smartRecoveryRequest{reason: "initial injected fault"})

	var first time.Time
	select {
	case first = <-calls:
	case <-time.After(time.Second):
		t.Fatal("initial recovery did not run")
	}
	select {
	case second := <-calls:
		t.Fatalf("health polling bypassed the configured one-hour retry backoff after %s", second.Sub(first))
	case <-time.After(150 * time.Millisecond):
	}
}
