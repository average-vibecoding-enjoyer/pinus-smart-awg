/* SPDX-License-Identifier: MIT */

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/amnezia-vpn/amneziawg-windows-client/smart"
	"github.com/amnezia-vpn/amneziawg-windows/conf/dpapi"
)

const keyDescription = "Pinus Smart AWG preset signing key v1"

type envelope struct {
	Schema    int    `json:"schema"`
	KeyID     string `json:"key_id"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func main() {
	mode := flag.String("mode", "", "generate, public, export, or sign")
	keyPath := flag.String("key", "", "DPAPI-protected key path")
	inputPath := flag.String("input", "", "preset payload path")
	outputPath := flag.String("output", "", "signed envelope path")
	revision := flag.Uint64("revision", 0, "catalog revision for export")
	publishedAt := flag.String("published-at", "", "RFC3339 publication time for export")
	flag.Parse()

	var err error
	switch *mode {
	case "generate":
		err = generateKey(*keyPath)
	case "public":
		err = printPublicKey(*keyPath)
	case "export":
		err = exportBuiltins(*outputPath, *revision, *publishedAt)
	case "sign":
		err = signPayload(*keyPath, *inputPath, *outputPath)
	default:
		err = errors.New("-mode must be generate, public, export, or sign")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func exportBuiltins(outputPath string, revision uint64, publishedAt string) error {
	if outputPath == "" || revision == 0 || publishedAt == "" {
		return errors.New("-output, -revision, and -published-at are required")
	}
	parsedTime, err := time.Parse(time.RFC3339, publishedAt)
	if err != nil {
		return fmt.Errorf("parse -published-at: %w", err)
	}
	payload := smart.PresetPayload{
		Schema:      1,
		Revision:    revision,
		PublishedAt: parsedTime.UTC(),
		Services:    smart.BuiltinServiceCatalog(),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := smart.ValidatePresetPayload(data, parsedTime.Add(time.Minute)); err != nil {
		return err
	}
	if err := atomicWrite(outputPath, data); err != nil {
		return err
	}
	fmt.Println("revision=" + strconv.FormatUint(revision, 10))
	return nil
}

func generateKey(path string) error {
	if path == "" {
		return errors.New("-key is required")
	}
	if _, err := os.Stat(path); err == nil {
		return errors.New("refusing to overwrite an existing signing key")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return err
	}
	defer wipe(seed)
	protected, err := dpapi.Encrypt(seed, keyDescription)
	if err != nil {
		return err
	}
	defer wipe(protected)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(protected); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	printKeyMetadata(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
	return nil
}

func loadSeed(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("-key is required")
	}
	protected, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	defer wipe(protected)
	seed, err := dpapi.Decrypt(protected, keyDescription)
	if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		wipe(seed)
		return nil, errors.New("decrypted signing seed has the wrong size")
	}
	return seed, nil
}

func printPublicKey(path string) error {
	seed, err := loadSeed(path)
	if err != nil {
		return err
	}
	defer wipe(seed)
	printKeyMetadata(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
	return nil
}

func printKeyMetadata(publicKey ed25519.PublicKey) {
	digest := sha256.Sum256(publicKey)
	fmt.Printf("public_key_base64=%s\n", base64.StdEncoding.EncodeToString(publicKey))
	fmt.Printf("key_id=%s\n", hex.EncodeToString(digest[:8]))
}

func signPayload(keyPath, inputPath, outputPath string) error {
	if inputPath == "" || outputPath == "" {
		return errors.New("-input and -output are required")
	}
	payload, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	if _, err := smart.ValidatePresetPayload(payload, time.Now()); err != nil {
		return fmt.Errorf("refusing to sign invalid presets: %w", err)
	}
	seed, err := loadSeed(keyPath)
	if err != nil {
		return err
	}
	defer wipe(seed)
	privateKey := ed25519.NewKeyFromSeed(seed)
	defer wipe(privateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	digest := sha256.Sum256(publicKey)
	signed := envelope{
		Schema:    1,
		KeyID:     hex.EncodeToString(digest[:8]),
		Payload:   base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
	data, err := json.MarshalIndent(signed, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := atomicWrite(outputPath, data); err != nil {
		return err
	}
	printKeyMetadata(publicKey)
	return nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pinus-sign-*")
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

func wipe(data []byte) {
	for index := range data {
		data[index] = 0
	}
}
