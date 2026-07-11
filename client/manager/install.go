/* SPDX-License-Identifier: MIT
 *
 * Copyright (C) 2019-2022 WireGuard LLC. All Rights Reserved.
 */

package manager

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/amnezia-vpn/amneziawg-windows/conf"
	"github.com/amnezia-vpn/amneziawg-windows/services"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows-client/version"
)

var cachedServiceManager *mgr.Mgr

const managerServiceName = "PinusAWGNextManager"

// wintunDLLSHA256 is replaced at link time by the multi-architecture release
// builder. The default is the official Wintun 0.14.1 amd64 DLL.
var wintunDLLSHA256 = "E5DA8447DC2C320EDC0FC52FA01885C103DE8C118481F683643CACC3220DAFCE"

func serviceManager() (*mgr.Mgr, error) {
	if cachedServiceManager != nil {
		return cachedServiceManager, nil
	}
	m, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	cachedServiceManager = m
	return cachedServiceManager, nil
}

var ErrManagerAlreadyRunning = errors.New("Manager already installed and running")

func StopManagerForUpgrade() error {
	m, err := serviceManager()
	if err != nil {
		return err
	}
	service, err := m.OpenService(managerServiceName)
	if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
		return nil
	}
	if err != nil {
		return err
	}
	defer service.Close()

	status, err := service.Query()
	if err != nil {
		return err
	}
	if status.State == svc.Stopped {
		return nil
	}
	if status.State != svc.StopPending {
		if _, err = service.Control(svc.Stop); err != nil && err != windows.ERROR_SERVICE_NOT_ACTIVE {
			return err
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		status, err = service.Query()
		if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
			return nil
		}
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("timed out waiting for the previous Pinus Smart AWG service to stop")
}

func copyFileAtomic(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".pinus-install-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = io.Copy(temporary, input); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	_ = os.Chmod(temporaryPath, info.Mode().Perm())
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = os.Rename(temporaryPath, destination)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func verifyPinnedFile(path, expectedSHA256, displayName string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	actual := fmt.Sprintf("%X", digest.Sum(nil))
	if actual != expectedSHA256 {
		return fmt.Errorf("%s integrity check failed: got %s", displayName, actual)
	}
	return nil
}

func verifyWintunDLL(path string) error {
	return verifyPinnedFile(path, wintunDLLSHA256, "wintun.dll")
}

func installBundlePaths() (currentExecutable, engineSource, wintunSource string, err error) {
	currentExecutable, err = os.Executable()
	if err != nil {
		return "", "", "", err
	}
	currentExecutable, err = filepath.Abs(currentExecutable)
	if err != nil {
		return "", "", "", err
	}
	bundleDirectory := filepath.Dir(currentExecutable)
	return currentExecutable,
		filepath.Join(bundleDirectory, "amnezia-box.exe"),
		filepath.Join(bundleDirectory, "wintun.dll"), nil
}

func VerifyInstallBundle() error {
	_, engineSource, wintunSource, err := installBundlePaths()
	if err != nil {
		return err
	}
	if err := smart.VerifyEngine(engineSource); err != nil {
		return fmt.Errorf("verify bundled engine before install: %w", err)
	}
	if err := verifyWintunDLL(wintunSource); err != nil {
		return fmt.Errorf("verify bundled Wintun before install: %w", err)
	}
	return nil
}

func secureInstallDirectory(dataDirectory string) string {
	return filepath.Join(filepath.Dir(dataDirectory), "Versions", version.Number)
}

// StageSecureInstall moves the service executable and its privileged runtime
// dependencies out of the user-writable download folder before SCM launches them.
func StageSecureInstall() (string, error) {
	currentExecutable, engineSource, wintunSource, err := installBundlePaths()
	if err != nil {
		return "", err
	}
	if err := smart.VerifyEngine(engineSource); err != nil {
		return "", fmt.Errorf("verify bundled engine before install: %w", err)
	}
	if err := verifyWintunDLL(wintunSource); err != nil {
		return "", fmt.Errorf("verify bundled Wintun before install: %w", err)
	}
	dataDirectory, err := conf.RootDirectory(true)
	if err != nil {
		return "", err
	}
	installDirectory := secureInstallDirectory(dataDirectory)
	if err := os.MkdirAll(installDirectory, 0755); err != nil {
		return "", err
	}
	installedExecutable := filepath.Join(installDirectory, "PinusSmartAWG.exe")
	installedEngine := filepath.Join(installDirectory, "amnezia-box.exe")
	installedWintun := filepath.Join(installDirectory, "wintun.dll")
	if !strings.EqualFold(engineSource, installedEngine) {
		if err := copyFileAtomic(engineSource, installedEngine); err != nil {
			return "", fmt.Errorf("install amnezia-box.exe: %w", err)
		}
	}
	if err := smart.VerifyEngine(installedEngine); err != nil {
		return "", fmt.Errorf("verify installed engine: %w", err)
	}
	if !strings.EqualFold(wintunSource, installedWintun) {
		if err := copyFileAtomic(wintunSource, installedWintun); err != nil {
			return "", fmt.Errorf("install wintun.dll: %w", err)
		}
	}
	if err := verifyWintunDLL(installedWintun); err != nil {
		return "", fmt.Errorf("verify installed Wintun: %w", err)
	}
	if !strings.EqualFold(currentExecutable, installedExecutable) {
		if err := copyFileAtomic(currentExecutable, installedExecutable); err != nil {
			return "", fmt.Errorf("install PinusSmartAWG.exe: %w", err)
		}
	}
	return installedExecutable, nil
}

func InstallManager() error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	return InstallManagerFrom(path)
}

func InstallManagerFrom(path string) error {
	m, err := serviceManager()
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(path); statErr != nil || info.IsDir() {
		if statErr != nil {
			return statErr
		}
		return fmt.Errorf("manager executable is a directory: %s", path)
	}

	// TODO: Do we want to bail if executable isn't being run from the right location?

	serviceName := managerServiceName
	service, err := m.OpenService(serviceName)
	if err == nil {
		status, err := service.Query()
		if err != nil {
			service.Close()
			return err
		}
		if status.State != svc.Stopped {
			service.Close()
			if status.State == svc.StartPending {
				// We were *just* started by something else, so return success here, assuming the other program
				// starting this does the right thing. This can happen when, e.g., the updater relaunches the
				// manager service and then invokes amneziawg.exe to raise the UI.
				return nil
			}
			return ErrManagerAlreadyRunning
		}
		err = service.Delete()
		service.Close()
		if err != nil {
			return err
		}
		for {
			service, err = m.OpenService(serviceName)
			if err != nil {
				break
			}
			service.Close()
			time.Sleep(time.Second / 3)
		}
	}

	config := mgr.Config{
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		DisplayName:  "Pinus Smart AWG Manager",
	}

	service, err = m.CreateService(serviceName, path, config, "/managerservice")
	if err != nil {
		return err
	}
	service.Start()
	return service.Close()
}

func UninstallManager() error {
	m, err := serviceManager()
	if err != nil {
		return err
	}
	serviceName := managerServiceName
	service, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	service.Control(svc.Stop)
	err = service.Delete()
	err2 := service.Close()
	if err != nil {
		return err
	}
	return err2
}

func uninstallServiceIfPresent(m *mgr.Mgr, name string) error {
	service, err := m.OpenService(name)
	if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
		return nil
	}
	if err != nil {
		return err
	}
	_, _ = service.Control(svc.Stop)
	deleteErr := service.Delete()
	closeErr := service.Close()
	if deleteErr != nil && deleteErr != windows.ERROR_SERVICE_MARKED_FOR_DELETE {
		return deleteErr
	}
	return closeErr
}

func removeOtherInstalledVersionsFrom(versionsDirectory, currentDirectory string) error {
	entries, err := os.ReadDir(versionsDirectory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var removeErrors []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(versionsDirectory, entry.Name())
		if strings.EqualFold(filepath.Clean(candidate), filepath.Clean(currentDirectory)) {
			continue
		}
		if err := os.RemoveAll(candidate); err != nil {
			removeErrors = append(removeErrors, fmt.Errorf("remove old version %s: %w", entry.Name(), err))
		}
	}
	return errors.Join(removeErrors...)
}

func removeOtherInstalledVersions() error {
	dataDirectory, err := conf.RootDirectory(false)
	if err != nil {
		return err
	}
	currentExecutable, err := os.Executable()
	if err != nil {
		return err
	}
	currentDirectory, err := filepath.Abs(filepath.Dir(currentExecutable))
	if err != nil {
		return err
	}
	versionsDirectory := filepath.Join(filepath.Dir(dataDirectory), "Versions")
	return removeOtherInstalledVersionsFrom(versionsDirectory, currentDirectory)
}

// UninstallAllServices removes every service owned by this application. User
// profiles remain in the protected data directory so reinstalling is safe.
func UninstallAllServices() error {
	m, err := serviceManager()
	if err != nil {
		return err
	}
	names, err := m.ListServices()
	if err != nil {
		return err
	}

	var uninstallErrors []error
	for _, name := range names {
		if !strings.HasPrefix(name, "PinusAWGNextTunnel$") {
			continue
		}
		if err := uninstallServiceIfPresent(m, name); err != nil {
			uninstallErrors = append(uninstallErrors, fmt.Errorf("remove %s: %w", name, err))
		}
	}
	if err := uninstallServiceIfPresent(m, managerServiceName); err != nil {
		uninstallErrors = append(uninstallErrors, fmt.Errorf("remove %s: %w", managerServiceName, err))
	}
	if err := removeOtherInstalledVersions(); err != nil {
		uninstallErrors = append(uninstallErrors, err)
	}
	return errors.Join(uninstallErrors...)
}

func InstallTunnel(configPath string) error {
	m, err := serviceManager()
	if err != nil {
		return err
	}
	path, err := os.Executable()
	if err != nil {
		return err
	}
	wintunPath := filepath.Join(filepath.Dir(path), "wintun.dll")
	if err := verifyWintunDLL(wintunPath); err != nil {
		return fmt.Errorf("native AWG runtime is incomplete: %w", err)
	}

	name, err := conf.NameFromPath(configPath)
	if err != nil {
		return err
	}

	serviceName, err := services.ServiceNameOfTunnel(name)
	if err != nil {
		return err
	}
	service, err := m.OpenService(serviceName)
	if err == nil {
		status, err := service.Query()
		if err != nil && err != windows.ERROR_SERVICE_MARKED_FOR_DELETE {
			service.Close()
			return err
		}
		if status.State != svc.Stopped && err != windows.ERROR_SERVICE_MARKED_FOR_DELETE {
			service.Close()
			return errors.New("Tunnel already installed and running")
		}
		err = service.Delete()
		service.Close()
		if err != nil && err != windows.ERROR_SERVICE_MARKED_FOR_DELETE {
			return err
		}
		for {
			service, err = m.OpenService(serviceName)
			if err != nil && err != windows.ERROR_SERVICE_MARKED_FOR_DELETE {
				break
			}
			service.Close()
			time.Sleep(time.Second / 3)
		}
	}

	config := mgr.Config{
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		Dependencies: []string{"Nsi", "TcpIp"},
		DisplayName:  "Pinus Smart AWG Tunnel: " + name,
		SidType:      windows.SERVICE_SID_TYPE_UNRESTRICTED,
	}
	service, err = m.CreateService(serviceName, path, config, "/tunnelservice", configPath)
	if err != nil {
		return err
	}

	err = service.Start()
	go trackTunnelService(name, service) // Pass off reference to handle.
	return err
}

func UninstallTunnel(name string) error {
	m, err := serviceManager()
	if err != nil {
		return err
	}
	serviceName, err := services.ServiceNameOfTunnel(name)
	if err != nil {
		return err
	}
	service, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	service.Control(svc.Stop)
	err = service.Delete()
	err2 := service.Close()
	if err != nil && err != windows.ERROR_SERVICE_MARKED_FOR_DELETE {
		return err
	}
	return err2
}

func changeTunnelServiceConfigFilePath(name, oldPath, newPath string) {
	var err error
	defer func() {
		if err != nil {
			log.Printf("Unable to change tunnel service command line argument from %#q to %#q: %v", oldPath, newPath, err)
		}
	}()
	m, err := serviceManager()
	if err != nil {
		return
	}
	serviceName, err := services.ServiceNameOfTunnel(name)
	if err != nil {
		return
	}
	service, err := m.OpenService(serviceName)
	if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
		err = nil
		return
	} else if err != nil {
		return
	}
	defer service.Close()
	config, err := service.Config()
	if err != nil {
		return
	}
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	args, err := windows.DecomposeCommandLine(config.BinaryPathName)
	if err != nil || len(args) != 3 ||
		!strings.EqualFold(args[0], exePath) || args[1] != "/tunnelservice" || !strings.EqualFold(args[2], oldPath) {
		err = nil
		return
	}
	args[2] = newPath
	config.BinaryPathName = windows.ComposeCommandLine(args)
	err = service.UpdateConfig(config)
}
