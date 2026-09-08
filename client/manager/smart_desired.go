/* SPDX-License-Identifier: MIT */

package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

const (
	smartDesiredVersion = 1
	smartDesiredMaxSize = 2 * smart.MaxSettingsSize
)

type smartDesiredState struct {
	Version    int                   `json:"version"`
	TunnelName string                `json:"tunnel_name"`
	Settings   smart.RoutingSettings `json:"settings"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

var smartDesiredLock sync.Mutex

func normalizeSmartDesired(state smartDesiredState) (smartDesiredState, error) {
	state.TunnelName = strings.TrimSpace(state.TunnelName)
	if !conf.TunnelNameIsValid(state.TunnelName) {
		return state, errors.New("invalid desired tunnel name")
	}
	settings, err := state.Settings.Normalized()
	if err != nil {
		return state, err
	}
	if usesNativeTunnel(settings) {
		return state, errors.New("native all-traffic tunnels do not use smart recovery")
	}
	state.Version = smartDesiredVersion
	state.Settings = settings
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	} else {
		state.UpdatedAt = state.UpdatedAt.UTC()
	}
	return state, nil
}

func smartDesiredPath() (string, error) {
	root, err := smart.EnsureWorkDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "desired.json"), nil
}

func writeSmartDesiredFile(path string, state smartDesiredState) error {
	state, err := normalizeSmartDesired(state)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > smartDesiredMaxSize {
		return errors.New("desired state is too large")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pinus-desired-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	_ = temporary.Chmod(0600)
	if _, err = temporary.Write(append(data, '\n')); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func readSmartDesiredFile(path string) (smartDesiredState, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return smartDesiredState{}, false, nil
	}
	if err != nil {
		return smartDesiredState{}, false, err
	}
	defer file.Close()
	limited := io.LimitReader(file, smartDesiredMaxSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return smartDesiredState{}, false, err
	}
	if len(data) > smartDesiredMaxSize {
		return smartDesiredState{}, false, errors.New("desired state file is too large")
	}
	var state smartDesiredState
	if err = json.Unmarshal(data, &state); err != nil {
		return smartDesiredState{}, false, fmt.Errorf("decode desired state: %w", err)
	}
	if state.Version != smartDesiredVersion {
		return smartDesiredState{}, false, fmt.Errorf("unsupported desired state version %d", state.Version)
	}
	state, err = normalizeSmartDesired(state)
	if err != nil {
		return smartDesiredState{}, false, fmt.Errorf("validate desired state: %w", err)
	}
	return state, true, nil
}

func saveSmartDesired(tunnelName string, settings smart.RoutingSettings) error {
	path, err := smartDesiredPath()
	if err != nil {
		return err
	}
	smartDesiredLock.Lock()
	defer smartDesiredLock.Unlock()
	return writeSmartDesiredFile(path, smartDesiredState{
		TunnelName: tunnelName,
		Settings:   settings,
		UpdatedAt:  time.Now().UTC(),
	})
}

func loadSmartDesired() (smartDesiredState, bool, error) {
	path, err := smartDesiredPath()
	if err != nil {
		return smartDesiredState{}, false, err
	}
	smartDesiredLock.Lock()
	defer smartDesiredLock.Unlock()
	return readSmartDesiredFile(path)
}

func clearSmartDesired() error {
	path, err := smartDesiredPath()
	if err != nil {
		return err
	}
	smartDesiredLock.Lock()
	defer smartDesiredLock.Unlock()
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func clearSmartDesiredForTunnel(tunnelName string) error {
	path, err := smartDesiredPath()
	if err != nil {
		return err
	}
	smartDesiredLock.Lock()
	defer smartDesiredLock.Unlock()
	state, exists, err := readSmartDesiredFile(path)
	if err != nil || !exists || !strings.EqualFold(state.TunnelName, tunnelName) {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
