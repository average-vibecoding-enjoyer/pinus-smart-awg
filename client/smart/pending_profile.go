package smart

import (
	"encoding/json"
	"errors"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
	"github.com/amnezia-vpn/amneziawg-windows/conf/dpapi"
	"os"
	"path/filepath"
)

type pendingProfile struct{ Name, Config string }

func pendingProfilePath(name string) string {
	return filepath.Join(UserSettingsDir(), SafeTunnelFileStem(name)+".pending.dpapi")
}
func SavePendingProfile(original string, config *conf.Config) error {
	if config == nil || !conf.TunnelNameIsValid(original) || !conf.TunnelNameIsValid(config.Name) {
		return errors.New("некорректный профиль")
	}
	raw, err := json.Marshal(pendingProfile{config.Name, config.ToWgQuick()})
	if err != nil {
		return err
	}
	defer clear(raw)
	encrypted, err := dpapi.Encrypt(raw, "PinusPreview.Pending:"+original)
	if err != nil {
		return err
	}
	return writeAtomic(pendingProfilePath(original), encrypted)
}
func LoadPendingProfile(original string) (*conf.Config, error) {
	encrypted, err := os.ReadFile(pendingProfilePath(original))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(encrypted) > 2*1024*1024 {
		return nil, errors.New("отложенный профиль слишком велик")
	}
	raw, err := dpapi.Decrypt(encrypted, "PinusPreview.Pending:"+original)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var pending pendingProfile
	if err = json.Unmarshal(raw, &pending); err != nil {
		return nil, err
	}
	return conf.FromWgQuick(pending.Config, pending.Name)
}
func DeletePendingProfile(name string) error {
	err := os.Remove(pendingProfilePath(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
