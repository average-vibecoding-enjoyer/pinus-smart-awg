/* SPDX-License-Identifier: MIT */

package manager

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

type smartProcessState struct {
	tunnelName string
	settings   smart.RoutingSettings
	process    *os.Process
	pid        int
	configPath string
	logPath    string
	waitOnce   sync.Once
	waitErr    error
}

var (
	smartProcessLock   sync.Mutex
	smartLifecycleLock sync.Mutex
	smartProcess       *smartProcessState
	smartJob           windows.Handle
)

const (
	smartLogMaxSize      = 5 * 1024 * 1024
	smartStartupTimeout  = 8 * time.Second
	smartReadinessPoll   = 100 * time.Millisecond
	smartReadinessSettle = 350 * time.Millisecond
)

var errSmartProcessExitedBeforeReady = errors.New("smart engine exited before the TUN interface became ready")

type smartInterfaceProbe func(string) (*net.Interface, error)
type smartLogProbe func() bool

func waitForSmartInterface(waitDone <-chan struct{}, probe smartInterfaceProbe, logProbe smartLogProbe, timeout, poll, settle time.Duration) error {
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()
	pollTicker := time.NewTicker(poll)
	defer pollTicker.Stop()
	for {
		select {
		case <-waitDone:
			return errSmartProcessExitedBeforeReady
		case <-timeoutTimer.C:
			return fmt.Errorf("timed out waiting for %s", smart.TunInterfaceName)
		case <-pollTicker.C:
			interfaze, err := probe(smart.TunInterfaceName)
			interfaceReady := err == nil && interfaze != nil && interfaze.Flags&net.FlagUp != 0
			logReady := logProbe != nil && logProbe()
			if !interfaceReady && !logReady {
				continue
			}
			settleTimer := time.NewTimer(settle)
			select {
			case <-waitDone:
				if !settleTimer.Stop() {
					<-settleTimer.C
				}
				return errSmartProcessExitedBeforeReady
			case <-timeoutTimer.C:
				if !settleTimer.Stop() {
					<-settleTimer.C
				}
				return fmt.Errorf("timed out while stabilizing %s", smart.TunInterfaceName)
			case <-settleTimer.C:
				return nil
			}
		}
	}
}

func smartLogHasStartup(path string, offset int64) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(file, 256*1024))
	if err != nil {
		return false
	}
	interfaceMarker := []byte("inbound/tun[tun-in]: started at " + smart.TunInterfaceName)
	return bytes.Contains(data, interfaceMarker) || bytes.Contains(data, []byte("sing-box started"))
}

func ensureSmartJobLocked() (windows.Handle, error) {
	if smartJob != 0 {
		return smartJob, nil
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	smartJob = job
	return smartJob, nil
}

func assignSmartProcessToJob(process *os.Process) error {
	smartProcessLock.Lock()
	job, err := ensureSmartJobLocked()
	smartProcessLock.Unlock()
	if err != nil {
		return err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.AssignProcessToJobObject(job, handle)
}

func rotateSmartLog(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Size() < smartLogMaxSize {
		return err
	}
	previous := path + ".1"
	_ = os.Remove(previous)
	return os.Rename(path, previous)
}

// Each engine generation owns a distinct path so an older process cannot
// remove the replacement config while it is shutting down.
func reserveSmartRuntimeConfig(configDir, safeName string) (string, error) {
	file, err := os.CreateTemp(configDir, safeName+"-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func smartPaths(tunnelName string) (configPath, logPath string, err error) {
	root, err := smart.EnsureWorkDir()
	if err != nil {
		return "", "", err
	}
	configDir := filepath.Join(root, "configs")
	logDir := filepath.Join(root, "logs")
	if err = os.MkdirAll(configDir, 0700); err != nil {
		return
	}
	if err = os.MkdirAll(logDir, 0700); err != nil {
		return
	}
	safeName := smart.SafeTunnelFileStem(tunnelName)
	configPath, err = reserveSmartRuntimeConfig(configDir, safeName)
	if err != nil {
		return "", "", err
	}
	return configPath, filepath.Join(logDir, safeName+".log"), nil
}

func cleanupSmartConfigDirectory(configDir string) error {
	entries, err := os.ReadDir(configDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var result error
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(configDir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	return result
}

func cleanupStaleSmartConfigs() error {
	root, err := smart.EnsureWorkDir()
	if err != nil {
		return err
	}
	return cleanupSmartConfigDirectory(filepath.Join(root, "configs"))
}

func (state *smartProcessState) wait() error {
	state.waitOnce.Do(func() {
		if state.process == nil {
			return
		}
		waitState, err := state.process.Wait()
		if err == nil && waitState != nil && !waitState.Success() {
			err = fmt.Errorf("smart engine exited: %s", waitState.String())
		}
		state.waitErr = err
	})
	return state.waitErr
}

func detachSmartProcessLocked() *smartProcessState {
	state := smartProcess
	smartProcess = nil
	return state
}

func stopSmartProcessState(state *smartProcessState) error {
	if state == nil {
		return nil
	}
	defer func() { _ = os.Remove(state.configPath) }()
	if state.process == nil {
		return nil
	}
	killErr := state.process.Kill()
	_ = state.wait()
	if errors.Is(killErr, os.ErrProcessDone) {
		return nil
	}
	return killErr
}

func smartStateForTunnel(tunnelName string) (TunnelState, bool) {
	smartProcessLock.Lock()
	defer smartProcessLock.Unlock()
	if smartProcess != nil && smartProcess.tunnelName == tunnelName {
		if smartProcess.process != nil {
			return TunnelStarted, true
		}
	}
	return TunnelStopped, false
}

func smartAnyStarted() bool {
	smartProcessLock.Lock()
	defer smartProcessLock.Unlock()
	return smartProcess != nil && smartProcess.process != nil
}

func smartProcessHealthyFor(tunnelName string) bool {
	smartProcessLock.Lock()
	running := smartProcess != nil && smartProcess.process != nil && strings.EqualFold(smartProcess.tunnelName, tunnelName)
	smartProcessLock.Unlock()
	if !running {
		return false
	}
	interfaze, err := net.InterfaceByName(smart.TunInterfaceName)
	return err == nil && interfaze != nil && interfaze.Flags&net.FlagUp != 0
}

func smartStopUnlocked() error {
	smartProcessLock.Lock()
	state := detachSmartProcessLocked()
	smartProcessLock.Unlock()
	err := stopSmartProcessState(state)
	if state != nil && state.tunnelName != "" {
		IPCServerNotifyTunnelChange(state.tunnelName, TunnelStopped, err)
	}
	return err
}

func smartStop() error {
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	return smartStopUnlocked()
}

func smartStopTunnel(tunnelName string) (bool, error) {
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	smartProcessLock.Lock()
	matches := smartProcess != nil && smartProcess.process != nil && smartProcess.tunnelName == tunnelName
	smartProcessLock.Unlock()
	if !matches {
		return false, nil
	}
	return true, smartStopUnlocked()
}

func usesNativeTunnel(settings smart.RoutingSettings) bool {
	return smart.NormalizeMode(string(settings.Mode)) == smart.ModeAll && !smart.HasEnabledCustomRules(settings)
}

func (s *ManagerService) SmartStart(tunnelName, rawSettings string) error {
	if s.elevatedToken == 0 {
		return windows.ERROR_ACCESS_DENIED
	}
	settings, err := smart.ParseSettingsPayload(rawSettings)
	if err != nil {
		return err
	}
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	err = s.smartStartLocked(tunnelName, settings, true)
	if err != nil {
		requestSmartRecovery("manual start failed", false, 500*time.Millisecond)
	}
	return err
}

func (s *ManagerService) smartStartLocked(tunnelName string, settings smart.RoutingSettings, persistDesired bool) error {
	if usesNativeTunnel(settings) {
		if err := smartStopUnlocked(); err != nil {
			return fmt.Errorf("stop smart routing before native tunnel: %w", err)
		}
		if err := s.Start(tunnelName); err != nil {
			return err
		}
		if persistDesired {
			if err := clearSmartDesired(); err != nil {
				_ = s.stopNativeTunnel(tunnelName)
				return fmt.Errorf("clear smart recovery state: %w", err)
			}
			markSmartRecoveryHealthy()
		}
		return nil
	}
	c, err := conf.LoadFromName(tunnelName)
	if err != nil {
		return err
	}
	configBytes, err := smart.BuildConfig(c, settings)
	if err != nil {
		return err
	}
	configPath, logPath, err := smartPaths(tunnelName)
	if err != nil {
		return err
	}
	keepConfig := false
	defer func() {
		if !keepConfig {
			_ = os.Remove(configPath)
		}
	}()
	if err = os.WriteFile(configPath, append(configBytes, '\n'), 0600); err != nil {
		return err
	}
	enginePath, err := smart.FindEnginePath()
	if err != nil {
		return err
	}
	check := exec.Command(enginePath, "check", "-c", configPath, "--disable-color")
	check.Dir = filepath.Dir(enginePath)
	if output, checkErr := check.CombinedOutput(); checkErr != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = checkErr.Error()
		}
		return fmt.Errorf("routing config validation failed: %s", message)
	}
	_, priorSmartActive := smartStateForTunnel(tunnelName)
	priorState, priorStateErr := s.State(tunnelName)
	nativeWasRunning := !priorSmartActive && priorStateErr == nil && (priorState == TunnelStarted || priorState == TunnelStarting)

	if err := s.stopAllNativeTunnels(); err != nil {
		return err
	}
	restoreNative := func(cause error) error {
		if !nativeWasRunning {
			return cause
		}
		if restoreErr := s.Start(tunnelName); restoreErr != nil {
			return errors.Join(cause, fmt.Errorf("restore native AWG: %w", restoreErr))
		}
		return fmt.Errorf("%w; native AWG was restored", cause)
	}

	smartProcessLock.Lock()
	previousState := detachSmartProcessLocked()
	smartProcessLock.Unlock()
	stopErr := stopSmartProcessState(previousState)
	if previousState != nil && previousState.tunnelName != "" {
		IPCServerNotifyTunnelChange(previousState.tunnelName, TunnelStopped, stopErr)
	}
	if stopErr != nil {
		return restoreNative(fmt.Errorf("stop previous smart routing: %w", stopErr))
	}

	if err := rotateSmartLog(logPath); err != nil {
		return restoreNative(err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return restoreNative(err)
	}
	defer logFile.Close()
	logStartOffset := int64(0)
	if info, statErr := logFile.Stat(); statErr == nil {
		logStartOffset = info.Size()
	}

	IPCServerNotifyTunnelChange(tunnelName, TunnelStarting, nil)
	args := []string{enginePath, "run", "-c", configPath, "--disable-color"}
	attr := &os.ProcAttr{
		Dir:   filepath.Dir(enginePath),
		Files: []*os.File{nil, logFile, logFile},
	}
	process, err := os.StartProcess(enginePath, args, attr)
	if err != nil {
		IPCServerNotifyTunnelChange(tunnelName, TunnelStopped, err)
		return restoreNative(fmt.Errorf("start smart engine: %w", err))
	}
	if err = assignSmartProcessToJob(process); err != nil {
		_ = process.Kill()
		_, _ = process.Wait()
		IPCServerNotifyTunnelChange(tunnelName, TunnelStopped, err)
		return restoreNative(fmt.Errorf("attach smart engine to lifecycle job: %w", err))
	}

	state := &smartProcessState{
		tunnelName: tunnelName,
		settings:   settings,
		process:    process,
		pid:        process.Pid,
		configPath: configPath,
		logPath:    logPath,
	}
	waitDone := make(chan struct{})
	go func() {
		_ = state.wait()
		close(waitDone)
	}()
	readyErr := waitForSmartInterface(
		waitDone,
		net.InterfaceByName,
		func() bool { return smartLogHasStartup(logPath, logStartOffset) },
		smartStartupTimeout,
		smartReadinessPoll,
		smartReadinessSettle,
	)
	if readyErr != nil {
		_ = process.Kill()
		waitErr := state.wait()
		if errors.Is(readyErr, errSmartProcessExitedBeforeReady) && waitErr != nil {
			readyErr = waitErr
		}
		IPCServerNotifyTunnelChange(tunnelName, TunnelStopped, readyErr)
		return restoreNative(fmt.Errorf("start smart engine: %w", readyErr))
	}

	if persistDesired {
		if err = saveSmartDesired(tunnelName, settings); err != nil {
			_ = stopSmartProcessState(state)
			IPCServerNotifyTunnelChange(tunnelName, TunnelStopped, err)
			return fmt.Errorf("persist smart recovery state: %w", err)
		}
	}
	smartProcessLock.Lock()
	smartProcess = state
	smartProcessLock.Unlock()
	markSmartRecoveryHealthy()
	keepConfig = true
	// sing-box has parsed the file by this point. Remove the on-disk copy of
	// the private key; a failed deletion is retried when the process stops.
	_ = os.Remove(configPath)
	IPCServerNotifyTunnelChange(tunnelName, TunnelStarted, nil)

	go func(expectedState *smartProcessState) {
		<-waitDone
		waitErr := expectedState.wait()
		_ = os.Remove(expectedState.configPath)
		smartProcessLock.Lock()
		if smartProcess == expectedState {
			smartProcess = nil
			smartProcessLock.Unlock()
			if waitErr != nil {
				log.Printf("[%s] smart engine stopped: %v", expectedState.tunnelName, waitErr)
			}
			IPCServerNotifyTunnelChange(expectedState.tunnelName, TunnelStopped, waitErr)
			requestSmartRecovery("engine exited", false, 500*time.Millisecond)
			return
		}
		smartProcessLock.Unlock()
	}(state)

	return nil
}

func (s *ManagerService) SmartStop() error {
	if s.elevatedToken == 0 {
		return windows.ERROR_ACCESS_DENIED
	}
	smartLifecycleLock.Lock()
	defer smartLifecycleLock.Unlock()
	desiredErr := clearSmartDesired()
	stopErr := smartStopUnlocked()
	markSmartRecoveryHealthy()
	return errors.Join(desiredErr, stopErr)
}
