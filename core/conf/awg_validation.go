package conf

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseAWGRange validates before any device, socket or adapter is created.
func ParseAWGRange(value string, bits int) (uint64, uint64, error) {
	parts := strings.Split(value, "-")
	if len(parts) > 2 || len(parts) == 0 {
		return 0, 0, errors.New("expected an integer or min-max range")
	}
	vals := [2]uint64{}
	for i, part := range parts {
		if part == "" {
			return 0, 0, errors.New("empty range boundary")
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return 0, 0, errors.New("range must contain unsigned decimal integers")
			}
		}
		n, err := strconv.ParseUint(part, 10, bits)
		if err != nil {
			return 0, 0, errors.New("range boundary is too large")
		}
		vals[i] = n
	}
	if len(parts) == 1 {
		vals[1] = vals[0]
	}
	if vals[0] > vals[1] {
		return 0, 0, errors.New("range minimum exceeds maximum")
	}
	return vals[0], vals[1], nil
}

func ParseAWGBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "on", "true", "1", "yes", "t":
		return true, nil
	case "off", "false", "0", "no", "f":
		return false, nil
	default:
		return false, errors.New("expected on or off")
	}
}

// ValidateCPS accepts the upstream CPS grammar without allocating a packet or
// invoking an obfuscator. In particular negative lengths can panic upstream.
func ValidateCPS(spec string) error {
	if len(spec) > 256*1024 {
		return errors.New("CPS description is too large")
	}
	rest, size, count, timestamps := strings.TrimSpace(spec), 0, 0, 0
	for rest != "" {
		if rest[0] != '<' {
			return errors.New("CPS must consist of <tag> elements")
		}
		end := strings.IndexByte(rest, '>')
		if end < 0 {
			return errors.New("CPS has an unclosed tag")
		}
		parts := strings.Fields(rest[1:end])
		if len(parts) == 0 || len(parts) > 2 {
			return errors.New("invalid CPS tag")
		}
		tag := parts[0]
		arg := ""
		if len(parts) == 2 {
			arg = parts[1]
		}
		switch tag {
		case "r", "rc", "rd", "dz":
			low, high, err := ParseAWGRange(arg, 16)
			if err != nil || low != high || strings.Contains(arg, "-") {
				return errors.New("CPS length must be a non-negative integer up to 65535")
			}
			size += int(low)
		case "b":
			data := strings.TrimPrefix(arg, "0x")
			if data == "" || len(data)%2 != 0 {
				return errors.New("invalid CPS hexadecimal bytes")
			}
			if _, err := hex.DecodeString(data); err != nil {
				return errors.New("invalid CPS hexadecimal bytes")
			}
			size += len(data) / 2
		case "t":
			if arg != "" {
				return errors.New("CPS timestamp takes no argument")
			}
			size += 4
			timestamps++
			if timestamps > 1 {
				return errors.New("CPS allows one timestamp per packet")
			}
		case "d", "ds":
			if arg != "" {
				return errors.New("CPS data tag takes no argument")
			}
		default:
			return fmt.Errorf("unsupported CPS tag %q", tag)
		}
		if size > 65507 {
			return errors.New("CPS packet exceeds UDP payload limit")
		}
		count++
		if count > 4096 {
			return errors.New("too many CPS tags")
		}
		rest = strings.TrimSpace(rest[end+1:])
	}
	if count == 0 {
		return errors.New("empty CPS packet")
	}
	return nil
}

func (c *Config) ValidateAWG() error {
	i := &c.Interface
	if i.JunkPacketMinSize > i.JunkPacketMaxSize {
		return errors.New("Jmin must not exceed Jmax")
	}
	if i.JunkPacketMaxSize > 65507 {
		return errors.New("Jmax exceeds UDP payload limit")
	}
	padding := []uint16{i.InitPacketJunkSize, i.ResponsePacketJunkSize, i.CookieReplyPacketJunkSize, i.TransportPacketJunkSize}
	mtu := int(i.MTU)
	if mtu == 0 {
		mtu = 1420
	}
	payload := []int{148, 92, 64, mtu + 32 + 15}
	for n, p := range padding {
		if int(p)+payload[n] > 65507 {
			return fmt.Errorf("S%d and packet/MTU exceed UDP payload limit", n+1)
		}
		if !i.HeaderProtectionKey.IsZero() && p < 12 {
			return fmt.Errorf("S%d must be at least 12 with HeaderProtectionKey", n+1)
		}
	}
	headers := []string{i.InitPacketMagicHeader, i.ResponsePacketMagicHeader, i.UnderloadPacketMagicHeader, i.TransportPacketMagicHeader}
	ranges := [4][2]uint64{}
	for n, h := range headers {
		if h == "" {
			h = strconv.Itoa(n + 1)
		}
		lo, hi, err := ParseAWGRange(h, 32)
		if err != nil {
			return fmt.Errorf("H%d: %w", n+1, err)
		}
		ranges[n] = [2]uint64{lo, hi}
		for prev := 0; prev < n; prev++ {
			if lo <= ranges[prev][1] && hi >= ranges[prev][0] {
				return fmt.Errorf("H%d and H%d overlap", prev+1, n+1)
			}
		}
	}
	for name, value := range map[string]string{
		"ContentPaddingAddition": i.ContentPaddingAddition, "RekeyAfterTime": i.RekeyAfterTime,
		"RekeyTimeout": i.RekeyTimeout, "RejectAfterTime": i.RejectAfterTime,
		"KeepaliveTimeout": i.KeepaliveTimeout, "MaxHandshakeAttempts": i.MaxHandshakeAttempts,
	} {
		if value != "" {
			if _, _, err := ParseAWGRange(value, 16); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	for name, value := range map[string]string{"RandomTrailers": i.RandomTrailers, "DisableCookies": i.DisableCookies} {
		if value != "" {
			if _, err := ParseAWGBool(value); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	for _, peer := range c.Peers {
		if peer.PersistentKeepalive != "" {
			if _, err := parsePersistentKeepalive(peer.PersistentKeepalive); err != nil {
				return err
			}
		}
	}
	for key, spec := range i.IPackets {
		if key != "i1" && key != "i2" && key != "i3" && key != "i4" && key != "i5" {
			return errors.New("unknown CPS packet field")
		}
		if err := ValidateCPS(spec); err != nil {
			return fmt.Errorf("%s: %w", strings.ToUpper(key), err)
		}
	}
	return nil
}

func (c *Config) ProtocolDescription() string {
	i := c.Interface
	if !i.HeaderProtectionKey.IsZero() || i.ContentPaddingAddition != "" || i.RekeyAfterTime != "" || i.RekeyTimeout != "" || i.RejectAfterTime != "" || i.KeepaliveTimeout != "" || i.MaxHandshakeAttempts != "" || i.RandomTrailers != "" || i.DisableCookies != "" {
		return "AWG 3.1"
	}
	for _, p := range c.Peers {
		if strings.Contains(p.PersistentKeepalive, "-") {
			return "AWG 3.1"
		}
	}
	return "WireGuard / AWG 1.x–2.x (совместимый профиль)"
}
