package smart

import (
	"encoding/json"
	"errors"
)

// The diagnostic inbound cannot follow direct exceptions or a selected-mode
// final route. Its random credentials are ephemeral and never returned to UI.
func AddDiagnosticInbound(config []byte, port int, password string) ([]byte, error) {
	if port < 1 || port > 65535 || len(password) < 32 {
		return nil, errors.New("invalid diagnostic listener")
	}
	var root map[string]any
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	inbounds, ok := root["inbounds"].([]any)
	if !ok {
		return nil, errors.New("missing inbounds")
	}
	root["inbounds"] = append(inbounds, map[string]any{"type": "socks", "tag": "pinus-probe", "listen": "127.0.0.1", "listen_port": port, "users": []any{map[string]any{"username": "pinus-probe", "password": password}}})
	route, ok := root["route"].(map[string]any)
	if !ok {
		return nil, errors.New("missing routes")
	}
	rules, ok := route["rules"].([]any)
	if !ok {
		return nil, errors.New("missing rules")
	}
	route["rules"] = append([]any{map[string]any{"inbound": []string{"pinus-probe"}, "action": "route", "outbound": "awg-out"}}, rules...)
	return json.MarshalIndent(root, "", "  ")
}
