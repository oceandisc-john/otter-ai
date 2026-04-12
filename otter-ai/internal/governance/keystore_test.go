package governance

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerateKeys_NewKey(t *testing.T) {
	dir := t.TempDir()
	cs, err := LoadOrGenerateKeys(dir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeys: %v", err)
	}
	if len(cs.GetPublicKey()) == 0 {
		t.Error("public key is empty")
	}

	// Key file should exist
	keyPath := filepath.Join(dir, "otter.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("key file not created")
	}
}

func TestLoadOrGenerateKeys_LoadExisting(t *testing.T) {
	dir := t.TempDir()

	// Generate first
	cs1, _ := LoadOrGenerateKeys(dir)
	pub1 := cs1.GetPublicKey()

	// Load existing
	cs2, err := LoadOrGenerateKeys(dir)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeys (reload): %v", err)
	}
	pub2 := cs2.GetPublicKey()

	if hex.EncodeToString(pub1) != hex.EncodeToString(pub2) {
		t.Error("reloaded key should match original")
	}
}

func TestExportPublicKey(t *testing.T) {
	cs, _ := NewCryptoSystem()
	exported := ExportPublicKey(cs)
	if exported == "" {
		t.Error("exported key should not be empty")
	}
	// Should be valid hex
	_, err := hex.DecodeString(exported)
	if err != nil {
		t.Errorf("exported key is not valid hex: %v", err)
	}
}

func TestImportPublicKey_Valid(t *testing.T) {
	cs, _ := NewCryptoSystem()
	exported := ExportPublicKey(cs)

	imported, err := ImportPublicKey(exported)
	if err != nil {
		t.Fatalf("ImportPublicKey: %v", err)
	}

	original := cs.GetPublicKey()
	if hex.EncodeToString(imported) != hex.EncodeToString(original) {
		t.Error("imported key should match original")
	}
}

func TestImportPublicKey_InvalidHex(t *testing.T) {
	_, err := ImportPublicKey("not-hex!!!")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestNewCryptoSystemFromSeed(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}

	cs, err := NewCryptoSystemFromSeed(seed)
	if err != nil {
		t.Fatalf("NewCryptoSystemFromSeed: %v", err)
	}
	if len(cs.GetPublicKey()) == 0 {
		t.Error("public key should not be empty")
	}
}

func TestNewCryptoSystemFromSeed_Deterministic(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}

	cs1, _ := NewCryptoSystemFromSeed(seed)
	cs2, _ := NewCryptoSystemFromSeed(seed)

	if hex.EncodeToString(cs1.GetPublicKey()) != hex.EncodeToString(cs2.GetPublicKey()) {
		t.Error("same seed should produce same key")
	}
}

func TestNewCryptoSystemFromSeed_TooShort(t *testing.T) {
	_, err := NewCryptoSystemFromSeed([]byte("short"))
	if err == nil {
		t.Error("expected error for short seed")
	}
}

func TestRegenerateKeys(t *testing.T) {
	dir := t.TempDir()

	// Generate initial
	cs1, _ := LoadOrGenerateKeys(dir)
	pub1 := hex.EncodeToString(cs1.GetPublicKey())

	// Regenerate
	cs2, err := RegenerateKeys(dir)
	if err != nil {
		t.Fatalf("RegenerateKeys: %v", err)
	}
	pub2 := hex.EncodeToString(cs2.GetPublicKey())

	if pub1 == pub2 {
		t.Error("regenerated key should be different")
	}

	// Loading should get the new key
	cs3, _ := LoadOrGenerateKeys(dir)
	pub3 := hex.EncodeToString(cs3.GetPublicKey())
	if pub2 != pub3 {
		t.Error("loaded key should match regenerated key")
	}
}

func TestSavePrivateKey_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "deep", "nested")
	keyPath := filepath.Join(subdir, "otter.key")

	cs, _ := NewCryptoSystem()
	err := savePrivateKey(keyPath, cs)
	if err != nil {
		t.Fatalf("savePrivateKey: %v", err)
	}

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("key file should exist")
	}
}
