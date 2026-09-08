package smart

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var settingsFileMutex sync.Mutex

type SettingsRevision struct {
	At       time.Time
	Reason   string
	Settings RoutingSettings
}

func historyPath(name string) string { return settingsPath(name) + ".history.json" }

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pinus-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func LoadSettingsHistory(name string) ([]SettingsRevision, error) {
	data, err := readLimitedFile(historyPath(name), 13*1024*1024)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 13*1024*1024 {
		return nil, errors.New("история настроек слишком велика")
	}
	var history []SettingsRevision
	err = json.Unmarshal(data, &history)
	return history, err
}

func recordPreviousSettings(name string) error {
	data, err := readLimitedFile(settingsPath(name), MaxSettingsSize)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	previous, err := ParseSettingsPayload(string(data))
	if err != nil {
		return err
	}
	history, err := LoadSettingsHistory(name)
	if err != nil {
		return err
	}
	history = append(history, SettingsRevision{At: time.Now().UTC(), Reason: "До изменения настроек", Settings: previous})
	if len(history) > 12 {
		history = history[len(history)-12:]
	}
	data, err = json.Marshal(history)
	if err != nil {
		return err
	}
	return writeAtomic(historyPath(name), data)
}

func RestoreSettingsRevision(name string, index int) error {
	history, err := LoadSettingsHistory(name)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(history) {
		return errors.New("версия настроек не найдена")
	}
	return SaveSettings(name, history[index].Settings)
}

type UserPreferences struct {
	HoldConnection bool `json:"hold_connection"`
}

func LoadPreferences() UserPreferences {
	prefs := UserPreferences{HoldConnection: true}
	if data, err := os.ReadFile(filepath.Join(UserSettingsDir(), "preferences.json")); err == nil {
		_ = json.Unmarshal(data, &prefs)
	}
	return prefs
}
func SavePreferences(prefs UserPreferences) error {
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(UserSettingsDir(), "preferences.json"), data)
}
