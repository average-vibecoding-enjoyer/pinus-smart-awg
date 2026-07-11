/* SPDX-License-Identifier: MIT */

package smart

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testPresetKey() (ed25519.PublicKey, ed25519.PrivateKey, string) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	digest := sha256.Sum256(publicKey)
	return publicKey, privateKey, hex.EncodeToString(digest[:8])
}

func testPresetPayload(t *testing.T, revision uint64, now time.Time) []byte {
	t.Helper()
	data, err := json.Marshal(PresetPayload{
		Schema:      presetPayloadSchema,
		Revision:    revision,
		PublishedAt: now.Add(-time.Minute).UTC(),
		Services:    BuiltinServiceCatalog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func signPresetEnvelope(t *testing.T, payload []byte, privateKey ed25519.PrivateKey, keyID string) []byte {
	t.Helper()
	data, err := json.Marshal(presetEnvelope{
		Schema:    presetEnvelopeSchema,
		KeyID:     keyID,
		Payload:   base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func resetPresetCatalogForTest() {
	applyServiceCatalog(BuiltinServiceCatalog(), PresetStatus{Revision: builtinCatalogRevision, Source: "built-in"})
}

func TestVerifyPresetEnvelopeRejectsTampering(t *testing.T) {
	now := time.Now()
	publicKey, privateKey, keyID := testPresetKey()
	payload := testPresetPayload(t, 2, now)
	envelope := signPresetEnvelope(t, payload, privateKey, keyID)
	verified, err := verifyPresetEnvelope(envelope, publicKey, keyID, now)
	if err != nil || verified.Revision != 2 {
		t.Fatalf("valid envelope rejected: revision=%d err=%v", verified.Revision, err)
	}

	var decoded presetEnvelope
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Payload = base64.StdEncoding.EncodeToString(append(payload, ' '))
	tampered, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyPresetEnvelope(tampered, publicKey, keyID, now); err == nil {
		t.Fatal("tampered preset payload passed signature verification")
	}
}

func TestValidatePresetPayloadRejectsUnsafeProcess(t *testing.T) {
	now := time.Now()
	payload := PresetPayload{
		Schema:      presetPayloadSchema,
		Revision:    2,
		PublishedAt: now,
		Services:    BuiltinServiceCatalog(),
	}
	payload.Services[0].ProcessNames = append(payload.Services[0].ProcessNames, "svchost.exe")
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePresetPayload(data, now); err == nil {
		t.Fatal("unsafe system process was accepted")
	}
}

func TestValidatePresetPayloadRequiresBuiltInServices(t *testing.T) {
	now := time.Now()
	services := BuiltinServiceCatalog()
	services = services[1:]
	data, err := json.Marshal(PresetPayload{Schema: presetPayloadSchema, Revision: 2, PublishedAt: now, Services: services})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePresetPayload(data, now); err == nil {
		t.Fatal("catalog without a required built-in service was accepted")
	}
}

func TestUpdateServiceCatalogAppliesSignedNewerRevision(t *testing.T) {
	resetPresetCatalogForTest()
	defer resetPresetCatalogForTest()
	t.Setenv("APPDATA", t.TempDir())
	now := time.Now().UTC()
	publicKey, privateKey, keyID := testPresetKey()
	envelope := signPresetEnvelope(t, testPresetPayload(t, 2, now), privateKey, keyID)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(envelope)
	}))
	defer server.Close()

	changed, err := updateServiceCatalog(context.Background(), server.Client(), []string{server.URL}, publicKey, keyID, now)
	if err != nil || !changed {
		t.Fatalf("signed update failed: changed=%v err=%v", changed, err)
	}
	status := ServiceCatalogStatus()
	if status.Revision != 2 || status.Source != "signed update" || status.LastError != "" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if _, err := os.Stat(presetCachePath()); err != nil {
		t.Fatalf("verified catalog was not cached: %v", err)
	}
}

func TestUpdateServiceCatalogRejectsRollback(t *testing.T) {
	resetPresetCatalogForTest()
	defer resetPresetCatalogForTest()
	now := time.Now().UTC()
	applyServiceCatalog(BuiltinServiceCatalog(), PresetStatus{Revision: 3, Source: "verified cache"})
	publicKey, privateKey, keyID := testPresetKey()
	envelope := signPresetEnvelope(t, testPresetPayload(t, 2, now), privateKey, keyID)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(envelope)
	}))
	defer server.Close()

	changed, err := updateServiceCatalog(context.Background(), server.Client(), []string{server.URL}, publicKey, keyID, now)
	if err == nil || changed || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("rollback result: changed=%v err=%v", changed, err)
	}
	if status := ServiceCatalogStatus(); status.Revision != 3 {
		t.Fatalf("rollback changed active revision: %#v", status)
	}
}

func TestUpdateServiceCatalogRejectsRevisionReuse(t *testing.T) {
	resetPresetCatalogForTest()
	defer resetPresetCatalogForTest()
	now := time.Now().UTC()
	publicKey, privateKey, keyID := testPresetKey()
	original := signPresetEnvelope(t, testPresetPayload(t, 2, now), privateKey, keyID)
	applyServiceCatalog(BuiltinServiceCatalog(), PresetStatus{Revision: 2, Source: "verified cache", Digest: presetEnvelopeDigest(original)})

	payload := PresetPayload{Schema: presetPayloadSchema, Revision: 2, PublishedAt: now.Add(-time.Minute), Services: BuiltinServiceCatalog()}
	payload.Services[0].Description = "Другое содержимое с той же ревизией"
	changedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	conflicting := signPresetEnvelope(t, changedPayload, privateKey, keyID)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(conflicting)
	}))
	defer server.Close()

	changed, err := updateServiceCatalog(context.Background(), server.Client(), []string{server.URL}, publicKey, keyID, now)
	if err == nil || changed || !strings.Contains(err.Error(), "reused preset revision") {
		t.Fatalf("revision reuse result: changed=%v err=%v", changed, err)
	}
}

func TestCommittedPresetEnvelopeMatchesProductionKey(t *testing.T) {
	presetDirectory := filepath.Join("..", "..", "presets")
	data, err := os.ReadFile(filepath.Join(presetDirectory, "catalog.signed.json"))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := presetPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := verifyPresetEnvelope(data, publicKey, presetPublicKeyID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if payload.Revision != 2 || !reflect.DeepEqual(payload.Services, BuiltinServiceCatalog()) {
		t.Fatalf("unexpected committed preset catalog: revision=%d services=%d", payload.Revision, len(payload.Services))
	}
	source, err := os.ReadFile(filepath.Join(presetDirectory, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	sourcePayload, err := ValidatePresetPayload(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sourcePayload, payload) {
		t.Fatal("signed preset payload does not match presets/catalog.json")
	}
}

func TestLoadCachedServiceCatalogUsesOnlyVerifiedEnvelope(t *testing.T) {
	resetPresetCatalogForTest()
	defer resetPresetCatalogForTest()
	t.Setenv("APPDATA", t.TempDir())
	data, err := os.ReadFile(filepath.Join("..", "..", "presets", "catalog.signed.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := presetCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadCachedServiceCatalog(); err != nil {
		t.Fatal(err)
	}
	if status := ServiceCatalogStatus(); status.Revision != 2 || status.Source != "verified cache" {
		t.Fatalf("unexpected cached status: %#v", status)
	}

	data[len(data)/2] ^= 1
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadCachedServiceCatalog(); err == nil {
		t.Fatal("tampered cached catalog was accepted")
	}
}
