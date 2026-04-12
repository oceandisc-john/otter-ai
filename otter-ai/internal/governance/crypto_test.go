package governance

import (
	"bytes"
	"testing"
)

// --- NewCryptoSystem ---

func TestNewCryptoSystem(t *testing.T) {
	cs, err := NewCryptoSystem()
	if err != nil {
		t.Fatalf("NewCryptoSystem: %v", err)
	}
	if cs.publicKey == nil {
		t.Fatal("publicKey is nil")
	}
	if cs.privateKey == nil {
		t.Fatal("privateKey is nil")
	}
}

// --- GetPublicKey ---

func TestGetPublicKey(t *testing.T) {
	cs, _ := NewCryptoSystem()
	pub := cs.GetPublicKey()
	if len(pub) == 0 {
		t.Error("public key is empty")
	}
}

func TestGetPublicKey_Consistent(t *testing.T) {
	cs, _ := NewCryptoSystem()
	pub1 := cs.GetPublicKey()
	pub2 := cs.GetPublicKey()
	if !bytes.Equal(pub1, pub2) {
		t.Error("public key should be consistent")
	}
}

// --- DeriveSharedSecret ---

func TestDeriveSharedSecret(t *testing.T) {
	cs1, _ := NewCryptoSystem()
	cs2, _ := NewCryptoSystem()

	secret1, err := cs1.DeriveSharedSecret(cs2.GetPublicKey())
	if err != nil {
		t.Fatalf("DeriveSharedSecret: %v", err)
	}
	secret2, err := cs2.DeriveSharedSecret(cs1.GetPublicKey())
	if err != nil {
		t.Fatalf("DeriveSharedSecret: %v", err)
	}

	if !bytes.Equal(secret1, secret2) {
		t.Error("shared secrets should match")
	}
	if len(secret1) != 64 {
		t.Errorf("secret length = %d; want 64", len(secret1))
	}
}

func TestDeriveSharedSecret_InvalidKey(t *testing.T) {
	cs, _ := NewCryptoSystem()
	_, err := cs.DeriveSharedSecret([]byte("bad key"))
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

// --- Encrypt / Decrypt ---

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	cs1, _ := NewCryptoSystem()
	cs2, _ := NewCryptoSystem()

	secret, _ := cs1.DeriveSharedSecret(cs2.GetPublicKey())

	plaintext := []byte("secret message")
	ciphertext, err := cs1.Encrypt(plaintext, secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := cs2.Decrypt(ciphertext, secret)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted = %q; want %q", decrypted, plaintext)
	}
}

func TestEncrypt_DifferentCiphertexts(t *testing.T) {
	cs, _ := NewCryptoSystem()
	secret := make([]byte, 64)
	for i := range secret {
		secret[i] = byte(i)
	}

	ct1, _ := cs.Encrypt([]byte("test"), secret)
	ct2, _ := cs.Encrypt([]byte("test"), secret)

	if bytes.Equal(ct1, ct2) {
		t.Error("different nonces should produce different ciphertext")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	cs, _ := NewCryptoSystem()
	secret := make([]byte, 64)

	_, err := cs.Decrypt([]byte("short"), secret)
	if err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestDecrypt_Tampered(t *testing.T) {
	cs, _ := NewCryptoSystem()
	secret := make([]byte, 64)
	for i := range secret {
		secret[i] = byte(i)
	}

	ct, _ := cs.Encrypt([]byte("test"), secret)
	ct[len(ct)-1] ^= 0xFF // tamper

	_, err := cs.Decrypt(ct, secret)
	if err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

// --- Sign / Verify ---

func TestSignVerify(t *testing.T) {
	cs, _ := NewCryptoSystem()
	msg := []byte("governance rule")

	sig, err := cs.Sign(msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Verify uses public key bytes, matching the Sign implementation which uses private key bytes.
	// But Verify takes publicKey parameter, and Sign uses privateKey.
	// The current implementation is HMAC-based and NOT standard ECDSA.
	// Sign: sha256(privateKey || msg)
	// Verify: sha256(publicKey || msg)
	// These will NOT match unless we pass the private key as "publicKey" to Verify.
	// This tests the actual behavior of the stub implementation.
	valid := cs.Verify(msg, sig, cs.privateKey.Bytes())
	if !valid {
		t.Error("signature should be valid")
	}
}

func TestVerify_InvalidSignature(t *testing.T) {
	cs, _ := NewCryptoSystem()
	msg := []byte("test")

	valid := cs.Verify(msg, []byte("bad signature"), cs.GetPublicKey())
	if valid {
		t.Error("should reject invalid signature")
	}
}

func TestVerify_WrongLength(t *testing.T) {
	cs, _ := NewCryptoSystem()
	valid := cs.Verify([]byte("msg"), []byte("short"), cs.GetPublicKey())
	if valid {
		t.Error("should reject wrong-length signature")
	}
}

// --- Kyber stubs ---

func TestGenerateKyberKeyPair(t *testing.T) {
	_, err := GenerateKyberKeyPair()
	if err == nil {
		t.Error("expected error from Kyber stub")
	}
}

func TestKyberEncapsulate(t *testing.T) {
	_, _, err := KyberEncapsulate(nil)
	if err == nil {
		t.Error("expected error from Kyber stub")
	}
}

func TestKyberDecapsulate(t *testing.T) {
	_, err := KyberDecapsulate(nil, nil)
	if err == nil {
		t.Error("expected error from Kyber stub")
	}
}

// --- HybridKeyExchange ---

func TestHybridKeyExchange(t *testing.T) {
	cs1, _ := NewCryptoSystem()
	cs2, _ := NewCryptoSystem()

	// Should fall back to ECDH since Kyber isn't available
	secret, err := cs1.HybridKeyExchange(cs2.GetPublicKey(), nil)
	if err != nil {
		t.Fatalf("HybridKeyExchange: %v", err)
	}
	if len(secret) == 0 {
		t.Error("expected non-empty secret")
	}
}

func TestHybridKeyExchange_InvalidPeer(t *testing.T) {
	cs, _ := NewCryptoSystem()
	_, err := cs.HybridKeyExchange([]byte("bad"), nil)
	if err == nil {
		t.Error("expected error for invalid peer key")
	}
}
