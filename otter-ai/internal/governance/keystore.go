package governance

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrGenerateKeys loads keys from disk or generates new ones
func LoadOrGenerateKeys(dataDir string) (*CryptoSystem, error) {
	keyPath := filepath.Join(dataDir, "otter.key")

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil {
		return loadCryptoSystemFromBytes(data)
	}

	// Generate new key
	cs, err := NewCryptoSystem()
	if err != nil {
		return nil, err
	}

	// Save key for future use
	if err := savePrivateKey(keyPath, cs); err != nil {
		return nil, fmt.Errorf("failed to save key: %w", err)
	}

	return cs, nil
}

// loadCryptoSystemFromBytes loads a CryptoSystem from private key bytes
func loadCryptoSystemFromBytes(data []byte) (*CryptoSystem, error) {
	// Decode hex if necessary
	keyBytes := data
	if len(data) > 0 && data[0] != 0x30 { // Not DER format, try hex
		decoded, err := hex.DecodeString(string(data))
		if err == nil {
			keyBytes = decoded
		}
	}

	curve := ecdh.P256()
	privateKey, err := curve.NewPrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &CryptoSystem{
		privateKey: privateKey,
		publicKey:  privateKey.PublicKey(),
		curve:      curve,
	}, nil
}

// savePrivateKey saves the private key to disk
func savePrivateKey(path string, cs *CryptoSystem) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	// Export private key as hex
	keyHex := hex.EncodeToString(cs.privateKey.Bytes())

	// Write with restricted permissions
	return os.WriteFile(path, []byte(keyHex), 0600)
}

// ExportPublicKey exports the public key as hex string
func ExportPublicKey(cs *CryptoSystem) string {
	return hex.EncodeToString(cs.GetPublicKey())
}

// ImportPublicKey imports a public key from hex string
func ImportPublicKey(hexKey string) ([]byte, error) {
	return hex.DecodeString(hexKey)
}

// NewCryptoSystemFromSeed creates a deterministic key from a seed (for testing)
func NewCryptoSystemFromSeed(seed []byte) (*CryptoSystem, error) {
	if len(seed) < 32 {
		return nil, fmt.Errorf("seed must be at least 32 bytes")
	}

	curve := ecdh.P256()

	// Use seed as private key (this is deterministic but less secure)
	// In production, use proper key derivation
	privateKey, err := curve.NewPrivateKey(seed[:32])
	if err != nil {
		return nil, fmt.Errorf("failed to create private key from seed: %w", err)
	}

	return &CryptoSystem{
		privateKey: privateKey,
		publicKey:  privateKey.PublicKey(),
		curve:      curve,
	}, nil
}

// RegenerateKeys generates a new key pair (use with caution!)
func RegenerateKeys(dataDir string) (*CryptoSystem, error) {
	cs := &CryptoSystem{
		curve: ecdh.P256(),
	}

	privateKey, err := cs.curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	cs.privateKey = privateKey
	cs.publicKey = privateKey.PublicKey()

	// Save the new key
	keyPath := filepath.Join(dataDir, "otter.key")
	if err := savePrivateKey(keyPath, cs); err != nil {
		return nil, fmt.Errorf("failed to save new key: %w", err)
	}

	return cs, nil
}
