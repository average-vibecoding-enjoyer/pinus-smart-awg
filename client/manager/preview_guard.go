package manager

import (
	"fmt"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"strings"
)

// Read-only preflight. Preview never stops, edits, or adopts production services.
// Called again by recovery, not just by the UI, before every connection attempt.
func previewConnectionPreflight() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("проверка изоляции Preview: %w", err)
	}
	defer m.Disconnect()
	names, err := m.ListServices()
	if err != nil {
		return fmt.Errorf("проверка изоляции Preview: %w", err)
	}
	for _, name := range names {
		n := strings.ToLower(name)
		if !strings.HasPrefix(n, "pinus") || strings.HasPrefix(n, "pinusawgpreview") {
			continue
		}
		service, err := m.OpenService(name)
		if err != nil {
			return fmt.Errorf("невозможно проверить службу %s: %w", name, err)
		}
		state, err := service.Query()
		service.Close()
		if err != nil {
			return err
		}
		if state.State != svc.Stopped {
			return fmt.Errorf("Preview не подключается, пока запущена основная версия Pinus (%s). Текущее соединение сохранено", name)
		}
	}
	return nil
}
