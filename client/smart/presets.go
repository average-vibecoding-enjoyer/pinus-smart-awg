/* SPDX-License-Identifier: MIT */

package smart

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	presetEnvelopeSchema = 1
	presetPayloadSchema  = 1
	presetMaxEnvelope    = 512 * 1024
	presetMaxPayload     = 384 * 1024
	presetPublicKeyB64   = "s8yyOUUzCEhu17TizaGp9Ml/o9HvT9YM/bsACNF5T4s="
	presetPublicKeyID    = "434f57314cd1ee6c"
)

var presetUpdateURLs = []string{
	"https://raw.githubusercontent.com/average-vibecoding-enjoyer/pinus-smart-awg/main/presets/catalog.signed.json",
	"https://github.com/average-vibecoding-enjoyer/pinus-smart-awg/raw/refs/heads/main/presets/catalog.signed.json",
}

var presetIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

type PresetPayload struct {
	Schema      int       `json:"schema"`
	Revision    uint64    `json:"revision"`
	PublishedAt time.Time `json:"published_at"`
	Services    []Service `json:"services"`
}

type presetEnvelope struct {
	Schema    int    `json:"schema"`
	KeyID     string `json:"key_id"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("JSON contains multiple values")
		}
		return err
	}
	return nil
}

func presetPublicKey() (ed25519.PublicKey, error) {
	key, err := base64.StdEncoding.DecodeString(presetPublicKeyB64)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid embedded preset public key")
	}
	digest := sha256.Sum256(key)
	if hex.EncodeToString(digest[:8]) != presetPublicKeyID {
		return nil, errors.New("embedded preset key ID mismatch")
	}
	return ed25519.PublicKey(key), nil
}

func verifyPresetEnvelope(data []byte, publicKey ed25519.PublicKey, keyID string, now time.Time) (PresetPayload, error) {
	if len(data) == 0 || len(data) > presetMaxEnvelope {
		return PresetPayload{}, errors.New("preset envelope has an invalid size")
	}
	var envelope presetEnvelope
	if err := decodeStrictJSON(data, &envelope); err != nil {
		return PresetPayload{}, fmt.Errorf("decode preset envelope: %w", err)
	}
	if envelope.Schema != presetEnvelopeSchema || envelope.KeyID != keyID {
		return PresetPayload{}, errors.New("preset envelope schema or key ID mismatch")
	}
	payloadBytes, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil || len(payloadBytes) == 0 || len(payloadBytes) > presetMaxPayload {
		return PresetPayload{}, errors.New("preset payload has an invalid encoding or size")
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PresetPayload{}, errors.New("preset signature has an invalid encoding or size")
	}
	if !ed25519.Verify(publicKey, payloadBytes, signature) {
		return PresetPayload{}, errors.New("preset signature verification failed")
	}
	return ValidatePresetPayload(payloadBytes, now)
}

func presetEnvelopeDigest(data []byte) string {
	var envelope presetEnvelope
	if json.Unmarshal(data, &envelope) != nil {
		return ""
	}
	payload, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func ValidatePresetPayload(data []byte, now time.Time) (PresetPayload, error) {
	if len(data) == 0 || len(data) > presetMaxPayload {
		return PresetPayload{}, errors.New("preset payload has an invalid size")
	}
	var payload PresetPayload
	if err := decodeStrictJSON(data, &payload); err != nil {
		return PresetPayload{}, fmt.Errorf("decode preset payload: %w", err)
	}
	if payload.Schema != presetPayloadSchema {
		return PresetPayload{}, errors.New("unsupported preset payload schema")
	}
	if payload.Revision < builtinCatalogRevision {
		return PresetPayload{}, errors.New("preset revision is below the built-in floor")
	}
	if payload.PublishedAt.IsZero() || payload.PublishedAt.After(now.Add(24*time.Hour)) {
		return PresetPayload{}, errors.New("preset publication time is invalid")
	}
	if len(payload.Services) == 0 || len(payload.Services) > 64 {
		return PresetPayload{}, errors.New("preset service count is invalid")
	}

	seenIDs := make(map[string]bool, len(payload.Services))
	totalEntries := 0
	for index := range payload.Services {
		service := &payload.Services[index]
		if !presetIDPattern.MatchString(service.ID) || seenIDs[service.ID] {
			return PresetPayload{}, fmt.Errorf("service %d has an invalid or duplicate ID", index+1)
		}
		seenIDs[service.ID] = true
		if err := validatePresetText(service.Name, 1, 64); err != nil {
			return PresetPayload{}, fmt.Errorf("service %q name: %w", service.ID, err)
		}
		if err := validatePresetText(service.Description, 1, 160); err != nil {
			return PresetPayload{}, fmt.Errorf("service %q description: %w", service.ID, err)
		}
		if len(service.ProcessNames) > 64 || len(service.Domains) > 256 || len(service.DomainSuffixes) > 256 {
			return PresetPayload{}, fmt.Errorf("service %q has too many route entries", service.ID)
		}
		totalEntries += len(service.ProcessNames) + len(service.Domains) + len(service.DomainSuffixes)
		if totalEntries > 4096 || len(service.ProcessNames)+len(service.Domains)+len(service.DomainSuffixes) == 0 {
			return PresetPayload{}, errors.New("preset route entry count is invalid")
		}
		if err := validatePresetProcesses(service.ProcessNames); err != nil {
			return PresetPayload{}, fmt.Errorf("service %q processes: %w", service.ID, err)
		}
		if err := validatePresetDomains(service.Domains, false); err != nil {
			return PresetPayload{}, fmt.Errorf("service %q domains: %w", service.ID, err)
		}
		if err := validatePresetDomains(service.DomainSuffixes, true); err != nil {
			return PresetPayload{}, fmt.Errorf("service %q domain suffixes: %w", service.ID, err)
		}
	}
	for _, service := range builtinServiceCatalog {
		if !seenIDs[service.ID] {
			return PresetPayload{}, fmt.Errorf("required built-in service %q is missing", service.ID)
		}
	}
	return payload, nil
}

func validatePresetText(value string, minRunes, maxRunes int) error {
	if value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return errors.New("text is not normalized UTF-8")
	}
	count := 0
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New("control characters are not allowed")
		}
		count++
	}
	if count < minRunes || count > maxRunes {
		return errors.New("text length is outside the allowed range")
	}
	return nil
}

func validatePresetProcesses(values []string) error {
	blocked := map[string]bool{
		"csrss.exe": true, "lsass.exe": true, "services.exe": true,
		"smss.exe": true, "svchost.exe": true, "wininit.exe": true,
		"winlogon.exe": true,
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		lower := strings.ToLower(value)
		if value != strings.TrimSpace(value) || len(value) > 128 || strings.ContainsAny(value, `\/:*?"<>|`) || filepath.Base(value) != value || !strings.HasSuffix(lower, ".exe") || blocked[lower] || seen[lower] {
			return fmt.Errorf("invalid, unsafe, or duplicate process %q", value)
		}
		seen[lower] = true
	}
	return nil
}

func validatePresetDomains(values []string, suffix bool) error {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		normalizedValue := value
		if suffix {
			if !strings.HasPrefix(value, ".") {
				return fmt.Errorf("domain suffix %q must start with a dot", value)
			}
			normalizedValue = strings.TrimPrefix(value, ".")
		}
		normalized, err := normalizeDomain(normalizedValue)
		if err != nil || normalized != normalizedValue || seen[value] {
			return fmt.Errorf("invalid or duplicate domain %q", value)
		}
		seen[value] = true
	}
	return nil
}

func presetCachePath() string {
	return filepath.Join(UserSettingsDir(), "presets", "catalog.signed.json")
}

func LoadCachedServiceCatalog() (resultErr error) {
	defer func() {
		if resultErr != nil {
			recordServiceCatalogCheck(time.Now(), fmt.Errorf("cached presets: %w", resultErr))
		}
	}()
	data, err := os.ReadFile(presetCachePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	publicKey, err := presetPublicKey()
	if err != nil {
		return err
	}
	payload, err := verifyPresetEnvelope(data, publicKey, presetPublicKeyID, time.Now())
	if err != nil {
		return err
	}
	current := ServiceCatalogStatus()
	if payload.Revision < current.Revision {
		return errors.New("cached preset revision would roll the catalog back")
	}
	incomingDigest := presetEnvelopeDigest(data)
	if payload.Revision == current.Revision && current.Digest != "" && incomingDigest != current.Digest {
		return errors.New("cached preset revision was reused with different content")
	}
	applyServiceCatalog(payload.Services, PresetStatus{
		Revision:    payload.Revision,
		Source:      "verified cache",
		Digest:      incomingDigest,
		PublishedAt: payload.PublishedAt,
	})
	return nil
}

func writePresetCache(data []byte) error {
	path := presetCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pinus-presets-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	_ = temporary.Chmod(0600)
	if _, err = temporary.Write(data); err == nil {
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

func presetHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.ResponseHeaderTimeout = 8 * time.Second
	return &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || request.URL.Scheme != "https" {
				return errors.New("unsafe preset update redirect")
			}
			return nil
		},
	}
}

func UpdateServiceCatalog(ctx context.Context) (bool, error) {
	publicKey, err := presetPublicKey()
	if err != nil {
		recordServiceCatalogCheck(time.Now(), err)
		return false, err
	}
	return updateServiceCatalog(ctx, presetHTTPClient(), presetUpdateURLs, publicKey, presetPublicKeyID, time.Now())
}

func updateServiceCatalog(ctx context.Context, client *http.Client, urls []string, publicKey ed25519.PublicKey, keyID string, now time.Time) (bool, error) {
	var updateErr error
	for _, url := range urls {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if requestErr != nil {
			updateErr = errors.Join(updateErr, requestErr)
			continue
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "Pinus-Smart-AWG-Presets/1")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			updateErr = errors.Join(updateErr, fmt.Errorf("%s: %w", url, requestErr))
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, presetMaxEnvelope+1))
		closeErr := response.Body.Close()
		if readErr == nil {
			readErr = closeErr
		}
		if response.StatusCode != http.StatusOK {
			readErr = errors.Join(readErr, fmt.Errorf("%s returned HTTP %d", url, response.StatusCode))
		}
		if readErr != nil || len(data) > presetMaxEnvelope {
			if readErr == nil {
				readErr = errors.New("preset response is too large")
			}
			updateErr = errors.Join(updateErr, readErr)
			continue
		}
		payload, verifyErr := verifyPresetEnvelope(data, publicKey, keyID, now)
		if verifyErr != nil {
			updateErr = errors.Join(updateErr, fmt.Errorf("%s: %w", url, verifyErr))
			continue
		}
		current := ServiceCatalogStatus()
		incomingDigest := presetEnvelopeDigest(data)
		if payload.Revision < current.Revision {
			updateErr = errors.Join(updateErr, fmt.Errorf("%s attempted preset rollback from %d to %d", url, current.Revision, payload.Revision))
			continue
		}
		if payload.Revision == current.Revision {
			if current.Digest != "" && incomingDigest != current.Digest {
				updateErr = errors.Join(updateErr, fmt.Errorf("%s reused preset revision %d with different content", url, payload.Revision))
				continue
			}
			recordServiceCatalogCheck(now, nil)
			return false, nil
		}
		if cacheErr := writePresetCache(data); cacheErr != nil {
			updateErr = errors.Join(updateErr, cacheErr)
			continue
		}
		applyServiceCatalog(payload.Services, PresetStatus{
			Revision:    payload.Revision,
			Source:      "signed update",
			Digest:      incomingDigest,
			PublishedAt: payload.PublishedAt,
			LastChecked: now,
		})
		return true, nil
	}
	if updateErr == nil {
		updateErr = errors.New("no preset update URL is configured")
	}
	recordServiceCatalogCheck(now, updateErr)
	return false, updateErr
}
