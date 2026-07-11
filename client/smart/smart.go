/* SPDX-License-Identifier: MIT */

package smart

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/amnezia-vpn/amneziawg-windows/conf"
)

// EngineSHA256 is replaced at link time by the multi-architecture release
// builder. The default keeps local amd64 development builds usable.
var EngineSHA256 = "682DCDCAACF8CF74B97D35B61CC29F5167443382A134A9D93D63142BB57C6FFD"

const TunInterfaceName = "pinus-smart-next"

func programDataDir() string {
	if root := os.Getenv("ProgramData"); root != "" {
		return filepath.Join(root, "PinusSmartAWGNext")
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return filepath.Join(os.TempDir(), "PinusSmartAWGNext")
	}
	return filepath.Join(dir, "PinusSmartAWGNext")
}

func WorkDir() string {
	if root, err := conf.RootDirectory(false); err == nil && root != "" {
		return filepath.Join(root, "SmartRuntime")
	}
	return programDataDir()
}

func EnsureWorkDir() (string, error) {
	root, err := conf.RootDirectory(true)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "SmartRuntime")
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	return path, nil
}

func UserSettingsDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return filepath.Join(os.TempDir(), "PinusSmartAWGNext")
	}
	return filepath.Join(dir, "PinusSmartAWGNext")
}

func VerifyEngine(path string) error {
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
	if actual != EngineSHA256 {
		return fmt.Errorf("amnezia-box.exe integrity check failed: got %s", actual)
	}
	return nil
}

func FindEnginePath() (string, error) {
	if env := os.Getenv("PINUS_SMART_ENGINE"); env != "" {
		if err := VerifyEngine(env); err != nil {
			return "", err
		}
		return env, nil
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(dir, "amnezia-box.exe"),
			filepath.Join(filepath.Dir(dir), "engine", "amnezia-box.exe"),
			filepath.Join(filepath.Dir(filepath.Dir(dir)), "engine", "amnezia-box.exe"),
		}
		for _, candidate := range candidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				if verifyErr := VerifyEngine(candidate); verifyErr != nil {
					return "", verifyErr
				}
				return candidate, nil
			}
		}
	}
	return "", errors.New("amnezia-box.exe not found near amneziawg.exe and PINUS_SMART_ENGINE is not set")
}

func ipcidrs(values []conf.IPCidr) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.String())
	}
	return out
}

func firstNonZeroMTU(config *conf.Config) int {
	if config.Interface.MTU > 0 {
		return int(config.Interface.MTU)
	}
	return 1408
}

func addUint16(target map[string]any, key string, value uint16) {
	if value > 0 {
		target[key] = int(value)
	}
}

func addString(target map[string]any, key string, value string) {
	if value != "" {
		target[key] = value
	}
}

func addIPackets(target map[string]any, values map[string]string) {
	for key, value := range values {
		if value != "" {
			target[strings.ToLower(key)] = value
		}
	}
}

func awgEndpoint(config *conf.Config) (map[string]any, error) {
	if len(config.Interface.Addresses) == 0 {
		return nil, errors.New("smart mode requires at least one interface address")
	}
	if len(config.Peers) == 0 {
		return nil, errors.New("smart mode requires at least one peer")
	}

	peers := make([]map[string]any, 0, len(config.Peers))
	for _, peer := range config.Peers {
		if peer.Endpoint.IsEmpty() {
			return nil, errors.New("smart mode requires peer endpoint")
		}
		allowedIPs := ipcidrs(peer.AllowedIPs)
		if len(allowedIPs) == 0 {
			allowedIPs = []string{"0.0.0.0/0", "::/0"}
		}
		peerObject := map[string]any{
			"address":                       peer.Endpoint.Host,
			"port":                          int(peer.Endpoint.Port),
			"public_key":                    peer.PublicKey.String(),
			"allowed_ips":                   allowedIPs,
			"persistent_keepalive_interval": int(peer.PersistentKeepalive),
		}
		if !peer.PresharedKey.IsZero() {
			peerObject["preshared_key"] = peer.PresharedKey.String()
		}
		peers = append(peers, peerObject)
	}

	endpoint := map[string]any{
		"type":             "awg",
		"tag":              "awg-out",
		"useIntegratedTun": false,
		"private_key":      config.Interface.PrivateKey.String(),
		"address":          ipcidrs(config.Interface.Addresses),
		"mtu":              firstNonZeroMTU(config),
		"peers":            peers,
	}
	addUint16(endpoint, "jc", config.Interface.JunkPacketCount)
	addUint16(endpoint, "jmin", config.Interface.JunkPacketMinSize)
	addUint16(endpoint, "jmax", config.Interface.JunkPacketMaxSize)
	addUint16(endpoint, "s1", config.Interface.InitPacketJunkSize)
	addUint16(endpoint, "s2", config.Interface.ResponsePacketJunkSize)
	addUint16(endpoint, "s3", config.Interface.CookieReplyPacketJunkSize)
	addUint16(endpoint, "s4", config.Interface.TransportPacketJunkSize)
	addString(endpoint, "h1", config.Interface.InitPacketMagicHeader)
	addString(endpoint, "h2", config.Interface.ResponsePacketMagicHeader)
	addString(endpoint, "h3", config.Interface.UnderloadPacketMagicHeader)
	addString(endpoint, "h4", config.Interface.TransportPacketMagicHeader)
	addIPackets(endpoint, config.Interface.IPackets)
	return endpoint, nil
}

var localCIDRs = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"127.0.0.0/8",
	"224.0.0.0/4",
	"255.255.255.255/32",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
}

func dnsServers(config *conf.Config, detour string) []map[string]any {
	servers := make([]map[string]any, 0, len(config.Interface.DNS))
	for index, dns := range config.Interface.DNS {
		if dns == nil {
			continue
		}
		if ip := dns.To4(); ip != nil {
			servers = append(servers, map[string]any{
				"type":        "udp",
				"tag":         fmt.Sprintf("profile-dns-%d", index),
				"server":      net.IP(ip).String(),
				"server_port": 53,
				"detour":      detour,
			})
		}
	}
	if len(servers) == 0 {
		servers = append(servers, map[string]any{
			"type":        "udp",
			"tag":         "cloudflare",
			"server":      "1.1.1.1",
			"server_port": 53,
			"detour":      detour,
		})
	}
	return servers
}

func customRuleObject(rule CustomRule) map[string]any {
	object := map[string]any{
		"action":   "route",
		"outbound": string(rule.Target),
	}
	if rule.Target == TargetVPN {
		object["outbound"] = "awg-out"
	} else {
		object["outbound"] = "direct"
	}
	switch rule.Kind {
	case RuleApplication:
		if strings.ContainsAny(rule.Value, `\/:`) {
			object["process_path"] = []string{rule.Value}
		} else {
			object["process_name"] = []string{rule.Value}
		}
	case RuleDomain:
		object["domain"] = []string{rule.Value}
		object["domain_suffix"] = []string{"." + rule.Value}
	case RuleCIDR:
		object["ip_cidr"] = []string{rule.Value}
	}
	return object
}

func serviceRuleObjects(settings RoutingSettings, outbound string) []map[string]any {
	processes := make([]string, 0)
	domains := make([]string, 0)
	suffixes := make([]string, 0)
	processSet := make(map[string]bool)
	domainSet := make(map[string]bool)
	suffixSet := make(map[string]bool)
	for _, service := range selectedServices(settings.SelectedApps) {
		for _, process := range service.ProcessNames {
			if !processSet[process] {
				processSet[process] = true
				processes = append(processes, process)
			}
		}
		for _, domain := range service.Domains {
			if !domainSet[domain] {
				domainSet[domain] = true
				domains = append(domains, domain)
			}
		}
		for _, suffix := range service.DomainSuffixes {
			if !suffixSet[suffix] {
				suffixSet[suffix] = true
				suffixes = append(suffixes, suffix)
			}
			root := strings.TrimPrefix(suffix, ".")
			if root != "" && !domainSet[root] {
				domainSet[root] = true
				domains = append(domains, root)
			}
		}
	}
	rules := make([]map[string]any, 0, 2)
	if len(processes) > 0 {
		rules = append(rules, map[string]any{
			"process_name": processes,
			"action":       "route",
			"outbound":     outbound,
		})
	}
	if len(domains) > 0 || len(suffixes) > 0 {
		rules = append(rules, map[string]any{
			"domain":        domains,
			"domain_suffix": suffixes,
			"action":        "route",
			"outbound":      outbound,
		})
	}
	return rules
}

func BuildConfig(config *conf.Config, settings RoutingSettings) ([]byte, error) {
	settings, err := settings.Normalized()
	if err != nil {
		return nil, err
	}
	endpoint, err := awgEndpoint(config)
	if err != nil {
		return nil, err
	}
	// Keep DNS inside the encrypted tunnel in every split-routing mode. Domain
	// rules still decide where the resolved connection goes, while resolution
	// itself cannot be poisoned or delayed by the local network.
	servers := dnsServers(config, "awg-out")
	finalDNS := "cloudflare"
	if tag, ok := servers[0]["tag"].(string); ok {
		finalDNS = tag
	}

	rules := []map[string]any{
		{
			"action":  "sniff",
			"timeout": "300ms",
		},
		{
			"protocol": "dns",
			"action":   "hijack-dns",
		},
		{
			"ip_cidr":  localCIDRs,
			"action":   "route",
			"outbound": "direct",
		},
	}
	for _, rule := range settings.CustomRules {
		if rule.Enabled {
			rules = append(rules, customRuleObject(rule))
		}
	}
	finalOutbound := "awg-out"
	if settings.Mode == ModeSelected {
		rules = append(rules, serviceRuleObjects(settings, "awg-out")...)
		finalOutbound = "direct"
	}

	boxConfig := map[string]any{
		"log": map[string]any{
			"level":     "info",
			"timestamp": true,
		},
		"dns": map[string]any{
			"servers":         servers,
			"final":           finalDNS,
			"strategy":        "ipv4_only",
			"reverse_mapping": true,
			"cache_capacity":  4096,
		},
		"endpoints": []any{endpoint},
		"inbounds": []any{
			map[string]any{
				"type":                     "tun",
				"tag":                      "tun-in",
				"interface_name":           TunInterfaceName,
				"address":                  []string{"172.19.77.1/30"},
				"mtu":                      1400,
				"auto_route":               true,
				"strict_route":             true,
				"stack":                    "mixed",
				"endpoint_independent_nat": true,
			},
		},
		"outbounds": []any{
			map[string]any{
				"type": "direct",
				"tag":  "direct",
			},
		},
		"route": map[string]any{
			"default_domain_resolver": finalDNS,
			"rules":                   rules,
			"final":                   finalOutbound,
			"auto_detect_interface":   true,
		},
	}
	return json.MarshalIndent(boxConfig, "", "  ")
}
