/* SPDX-License-Identifier: MIT */

package smart

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/amnezia-vpn/amneziawg-windows/conf"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/idna"
)

const settingsVersion = 5
const MaxSettingsSize = 1024 * 1024

var legacyDefaultServiceIDs = []string{"discord", "youtube", "telegram", "instagram"}

type Mode string

const (
	ModeAll      Mode = "all"
	ModeSelected Mode = "selected"

	// Legacy values kept so profiles created by the prototype migrate cleanly.
	ModeBypass Mode = "bypass"
	ModeNative Mode = "native"
	ModeSocial Mode = "social"
)

var modeLabels = []string{
	"Весь интернет через VPN",
	"Только выбранное через VPN",
}

var modeValues = []Mode{ModeAll, ModeSelected}

type RuleKind string

const (
	RuleApplication RuleKind = "application"
	RuleDomain      RuleKind = "domain"
	RuleCIDR        RuleKind = "cidr"
)

type RouteTarget string

const (
	TargetVPN    RouteTarget = "vpn"
	TargetDirect RouteTarget = "direct"
)

type CustomRule struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Kind    RuleKind    `json:"kind"`
	Value   string      `json:"value"`
	Target  RouteTarget `json:"target"`
	Enabled bool        `json:"enabled"`
}

type RoutingSettings struct {
	ServiceRoutes    map[Mode]map[string]bool `json:"service_routes,omitempty"`
	CatalogEnvelope  json.RawMessage          `json:"catalog_envelope,omitempty"`
	FallbackProfiles []string                 `json:"fallback_profiles,omitempty"`
	Protection       string                   `json:"protection,omitempty"`
	Version          int                      `json:"version"`
	Mode             Mode                     `json:"mode"`
	SelectedApps     []string                 `json:"selected_apps"`
	CustomRules      []CustomRule             `json:"custom_rules,omitempty"`
}

func HasEnabledCustomRules(settings RoutingSettings) bool {
	for _, rule := range settings.CustomRules {
		if rule.Enabled {
			return true
		}
	}
	return false
}

func ModeLabels() []string {
	out := make([]string, len(modeLabels))
	copy(out, modeLabels)
	return out
}

func ModeFromIndex(index int) Mode {
	if index < 0 || index >= len(modeValues) {
		return ModeAll
	}
	return modeValues[index]
}

func IndexOfMode(mode Mode) int {
	mode = NormalizeMode(string(mode))
	for i, value := range modeValues {
		if value == mode {
			return i
		}
	}
	return 0
}

func NormalizeMode(mode string) Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(mode))) {
	case ModeSelected, ModeSocial:
		return ModeSelected
	case ModeBypass:
		return ModeAll
	case ModeNative:
		return ModeSelected
	default:
		return ModeAll
	}
}

func DefaultSettings() RoutingSettings {
	return RoutingSettings{
		Version:      settingsVersion,
		Mode:         ModeAll,
		SelectedApps: DefaultServiceIDs(),
	}
}

func selectedExactlyMatches(values, expected []string) bool {
	wanted := make(map[string]bool, len(expected))
	for _, value := range expected {
		wanted[value] = true
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !wanted[value] {
			return false
		}
		seen[value] = true
	}
	return len(seen) == len(wanted)
}

func NewRuleID() string {
	var value [6]byte
	if _, err := rand.Read(value[:]); err == nil {
		return "rule-" + hex.EncodeToString(value[:])
	}
	return "rule-local"
}

func normalizeDomain(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "*.")
	if value == "" {
		return "", errors.New("укажи домен вроде example.com")
	}
	parseValue := value
	if !strings.Contains(parseValue, "://") {
		parseValue = "//" + parseValue
	}
	parsed, err := url.Parse(parseValue)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return "", errors.New("укажи домен вроде example.com")
	}
	host := strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(parsed.Hostname()), "."), "*.")
	host, err = idna.Lookup.ToASCII(host)
	if err != nil || host == "" || len(host) > 253 || !strings.Contains(host, ".") {
		return "", errors.New("укажи домен вроде example.com")
	}
	if net.ParseIP(host) != nil {
		return "", errors.New("для IP-адреса выбери тип «IP / CIDR»")
	}
	return host, nil
}

func normalizeCIDR(value string) (string, error) {
	value = strings.TrimSpace(value)
	if ip := net.ParseIP(value); ip != nil {
		if ip.To4() != nil {
			return ip.String() + "/32", nil
		}
		return ip.String() + "/128", nil
	}
	ip, network, err := net.ParseCIDR(value)
	if err != nil {
		return "", errors.New("укажи IP или сеть, например 1.1.1.1 или 10.0.0.0/8")
	}
	network.IP = ip.Mask(network.Mask)
	return network.String(), nil
}

func normalizeApplication(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), "\"")
	if value == "" {
		return "", errors.New("выбери EXE-файл или укажи имя процесса")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("путь к приложению должен быть в одну строку")
	}
	if strings.ContainsAny(value, `\/:`) {
		value = filepath.Clean(value)
		if !strings.EqualFold(filepath.Ext(value), ".exe") {
			return "", errors.New("выбери исполняемый файл с расширением .exe")
		}
		return value, nil
	}
	value = filepath.Base(value)
	if !strings.HasSuffix(strings.ToLower(value), ".exe") {
		value += ".exe"
	}
	return value, nil
}

func (rule CustomRule) normalized(index int) (CustomRule, error) {
	rule.ID = strings.TrimSpace(rule.ID)
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%d", index+1)
	}
	rule.Name = strings.TrimSpace(rule.Name)
	switch rule.Kind {
	case RuleApplication:
		value, err := normalizeApplication(rule.Value)
		if err != nil {
			return rule, err
		}
		rule.Value = value
	case RuleDomain:
		value, err := normalizeDomain(rule.Value)
		if err != nil {
			return rule, err
		}
		rule.Value = value
	case RuleCIDR:
		value, err := normalizeCIDR(rule.Value)
		if err != nil {
			return rule, err
		}
		rule.Value = value
	default:
		return rule, errors.New("неизвестный тип правила")
	}
	if rule.Target != TargetVPN && rule.Target != TargetDirect {
		return rule, errors.New("неизвестное направление правила")
	}
	if rule.Name == "" {
		rule.Name = rule.Value
	}
	return rule, nil
}

func (settings RoutingSettings) Normalized() (RoutingSettings, error) {
	if settings.Version > settingsVersion {
		return settings, errors.New("unsupported settings version")
	}
	if settings.Protection != "" && settings.Protection != "all" {
		return settings, errors.New("unknown protection policy")
	}
	if len(settings.FallbackProfiles) > 8 {
		return settings, errors.New("не более 8 резервных профилей")
	}
	for _, name := range settings.FallbackProfiles {
		if !conf.TunnelNameIsValid(name) {
			return settings, errors.New("неверное имя резервного профиля")
		}
	}
	storedVersion := settings.Version
	legacyMode := settings.Mode == ModeSocial || settings.Mode == ModeNative
	settings.Mode = NormalizeMode(string(settings.Mode))
	if settings.Version == 0 {
		settings.Version = settingsVersion
		if settings.SelectedApps == nil || legacyMode {
			settings.SelectedApps = DefaultServiceIDs()
		}
	}
	if storedVersion > 0 && storedVersion < settingsVersion && selectedExactlyMatches(settings.SelectedApps, legacyDefaultServiceIDs) {
		settings.SelectedApps = append(settings.SelectedApps, AIServiceID)
	}
	settings.Version = settingsVersion

	catalog, catalogErr := verifiedSettingsCatalog(settings.CatalogEnvelope)
	if catalogErr != nil {
		return settings, catalogErr
	}
	known := make(map[string]bool)
	for _, service := range catalog {
		known[service.ID] = true
	}
	selected := make([]string, 0, len(settings.SelectedApps))
	seen := make(map[string]bool)
	for _, id := range settings.SelectedApps {
		id = strings.ToLower(strings.TrimSpace(id))
		if known[id] && !seen[id] {
			seen[id] = true
			selected = append(selected, id)
		}
	}
	settings.SelectedApps = selected

	rules := make([]CustomRule, 0, len(settings.CustomRules))
	ids := make(map[string]bool)
	for i, rule := range settings.CustomRules {
		normalized, err := rule.normalized(i)
		if err != nil {
			return settings, fmt.Errorf("правило %d: %w", i+1, err)
		}
		if ids[normalized.ID] {
			normalized.ID = fmt.Sprintf("%s-%d", normalized.ID, i+1)
		}
		ids[normalized.ID] = true
		rules = append(rules, normalized)
	}
	settings.CustomRules = rules
	return settings, nil
}

func legacyTunnelFileStem(tunnelName string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			return r
		}
		return '_'
	}, tunnelName)
}

// SafeTunnelFileStem keeps runtime filenames readable while the hash prevents
// collisions between Unicode names that would otherwise all become underscores.
func SafeTunnelFileStem(tunnelName string) string {
	safe := strings.Trim(legacyTunnelFileStem(strings.TrimSpace(tunnelName)), "._-")
	if safe == "" {
		safe = "profile"
	}
	if len(safe) > 48 {
		safe = safe[:48]
	}
	digest := sha256.Sum256([]byte(tunnelName))
	return safe + "-" + hex.EncodeToString(digest[:6])
}

func settingsPath(tunnelName string) string {
	return filepath.Join(UserSettingsDir(), "tunnels", SafeTunnelFileStem(tunnelName)+".json")
}

func legacySettingsPath(tunnelName string) string {
	return filepath.Join(UserSettingsDir(), "tunnels", legacyTunnelFileStem(tunnelName)+".json")
}

func ParseSettingsPayload(raw string) (RoutingSettings, error) {
	if len(raw) > MaxSettingsSize {
		return RoutingSettings{}, errors.New("настройки превышают 1 МБ")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		settings := DefaultSettings()
		settings.Mode = NormalizeMode(raw)
		return settings.Normalized()
	}
	var settings RoutingSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return RoutingSettings{}, fmt.Errorf("не удалось прочитать настройки маршрутизации: %w", err)
	}
	return settings.Normalized()
}

func SettingsPayload(settings RoutingSettings) (string, error) {
	normalized, err := settings.Normalized()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	if len(data) > MaxSettingsSize {
		return "", errors.New("настройки превышают 1 МБ")
	}
	return string(data), nil
}

func LoadSettings(tunnelName string) (RoutingSettings, error) {
	path := settingsPath(tunnelName)
	data, err := readLimitedFile(path, MaxSettingsSize)
	legacyPath := legacySettingsPath(tunnelName)
	loadedLegacy := false
	if errors.Is(err, os.ErrNotExist) && legacyPath != path {
		data, err = readLimitedFile(legacyPath, MaxSettingsSize)
		loadedLegacy = err == nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return RoutingSettings{}, err
	}
	storedVersion := 0
	var metadata struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(data, &metadata) == nil {
		storedVersion = metadata.Version
	}
	settings, err := ParseSettingsPayload(string(data))
	if err != nil {
		return RoutingSettings{}, err
	}
	if loadedLegacy || storedVersion != settings.Version {
		if saveErr := SaveSettings(tunnelName, settings); saveErr == nil {
			if loadedLegacy {
				_ = os.Remove(legacyPath)
			}
		}
	}
	return settings, nil
}

func SaveSettings(tunnelName string, settings RoutingSettings) error {
	settingsFileMutex.Lock()
	defer settingsFileMutex.Unlock()
	normalized, err := settings.Normalized()
	if err != nil {
		return err
	}
	path := settingsPath(tunnelName)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	if len(data) >= MaxSettingsSize {
		return errors.New("настройки превышают 1 МБ")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pinus-settings-*")
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
	if err := recordPreviousSettings(tunnelName); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func DeleteSettings(tunnelName string) error {
	paths := []string{settingsPath(tunnelName), legacySettingsPath(tunnelName)}
	var result error
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	return result
}

func LoadMode(tunnelName string) Mode {
	settings, err := LoadSettings(tunnelName)
	if err != nil {
		return ModeAll
	}
	return settings.Mode
}

func SaveMode(tunnelName string, mode Mode) error {
	settings, err := LoadSettings(tunnelName)
	if err != nil {
		settings = DefaultSettings()
	}
	settings.Mode = NormalizeMode(string(mode))
	return SaveSettings(tunnelName, settings)
}
