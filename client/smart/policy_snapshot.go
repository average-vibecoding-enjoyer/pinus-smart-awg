package smart

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

// CaptureCatalog pins the signed catalog to an explicit settings revision.
// Recovery and the manager compile this snapshot, never a different process's
// mutable UI catalog. No HTTP request is performed here.
func CaptureCatalog(settings RoutingSettings) (RoutingSettings, error) {
	data, err := os.ReadFile(presetCachePath())
	if errors.Is(err, os.ErrNotExist) {
		settings.CatalogEnvelope = nil
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if _, err = verifiedSettingsCatalog(data); err != nil {
		return settings, err
	}
	settings.CatalogEnvelope = append(json.RawMessage(nil), data...)
	return settings, nil
}

func verifiedSettingsCatalog(envelope []byte) ([]Service, error) {
	if len(envelope) == 0 {
		return BuiltinServiceCatalog(), nil
	}
	key, err := presetPublicKey()
	if err != nil {
		return nil, err
	}
	payload, err := verifyPresetEnvelope(envelope, key, presetPublicKeyID, time.Now())
	if err != nil {
		return nil, err
	}
	return cloneServices(payload.Services), nil
}

func (settings RoutingSettings) SelectedServices() []Service {
	catalog, err := verifiedSettingsCatalog(settings.CatalogEnvelope)
	if err != nil {
		return nil
	} // BuildConfig validates before reaching this method.
	ids := map[string]bool{}
	for _, id := range settings.SelectedApps {
		ids[id] = true
	}
	selected := make([]Service, 0, len(ids))
	for _, service := range catalog {
		if ids[service.ID] {
			selected = append(selected, service)
		}
	}
	return selected
}
