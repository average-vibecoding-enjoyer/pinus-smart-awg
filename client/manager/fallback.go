package manager

import (
	"errors"
	"fmt"
	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/tunnel/firewall"
	"golang.org/x/sys/windows"
	"strings"
)

func recoverWithFallbacks(primary string, settings smart.RoutingSettings, start func(string) error) error {
	names := append([]string{primary}, settings.FallbackProfiles...)
	seen := map[string]bool{}
	var result error
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		err := start(name)
		if err == nil {
			return nil
		}
		result = errors.Join(result, fmt.Errorf("%s: %w", name, err))
	}
	return result
}
func ClearPreviewProtection() error { return firewall.ClearPreviewGuard() }
func InstalledManagerExecutable() (string, error) {
	m, err := serviceManager()
	if err != nil {
		return "", err
	}
	service, err := m.OpenService(managerServiceName)
	if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer service.Close()
	config, err := service.Config()
	if err != nil {
		return "", err
	}
	args, err := windows.DecomposeCommandLine(config.BinaryPathName)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", errors.New("empty installed service command")
	}
	return args[0], nil
}
