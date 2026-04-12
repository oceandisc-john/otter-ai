package api

import (
	"testing"
	"time"
)

// --- NewJWTManager ---

func TestNewJWTManager_WithSecret(t *testing.T) {
	m, err := NewJWTManager("test-secret")
	if err != nil {
		t.Fatalf("NewJWTManager: %v", err)
	}
	if m == nil {
		t.Fatal("manager is nil")
	}
}

func TestNewJWTManager_EmptySecret(t *testing.T) {
	m, err := NewJWTManager("")
	if err != nil {
		t.Fatalf("NewJWTManager: %v", err)
	}
	if m == nil {
		t.Fatal("manager is nil")
	}
	// Should auto-generate a secret
	if len(m.secretKey) == 0 {
		t.Error("secret should be auto-generated")
	}
}

// --- GenerateToken / ValidateToken ---

func TestGenerateAndValidateToken(t *testing.T) {
	m, _ := NewJWTManager("test-secret")

	token, err := m.GenerateToken("user1")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}

	claims, err := m.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != "user1" {
		t.Errorf("UserID = %q; want user1", claims.UserID)
	}
	if claims.Issuer != JWTIssuer {
		t.Errorf("Issuer = %q; want %s", claims.Issuer, JWTIssuer)
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	m, _ := NewJWTManager("test-secret")
	_, err := m.ValidateToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	m1, _ := NewJWTManager("secret-1")
	m2, _ := NewJWTManager("secret-2")

	token, _ := m1.GenerateToken("user")
	_, err := m2.ValidateToken(token)
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestValidateToken_TokenFields(t *testing.T) {
	m, _ := NewJWTManager("secret")
	token, _ := m.GenerateToken("test-user")
	claims, _ := m.ValidateToken(token)

	if claims.ExpiresAt == nil {
		t.Error("ExpiresAt should be set")
	} else {
		expiry := claims.ExpiresAt.Time
		expected := time.Now().Add(JWTExpirationTime)
		// Allow 5 second margin
		if expiry.Before(expected.Add(-5*time.Second)) || expiry.After(expected.Add(5*time.Second)) {
			t.Errorf("expiry %v not within expected range of %v", expiry, expected)
		}
	}
}

// --- generateRandomSecret ---

func TestGenerateRandomSecret(t *testing.T) {
	s1 := generateRandomSecret()
	s2 := generateRandomSecret()
	if s1 == "" || s2 == "" {
		t.Error("secrets should not be empty")
	}
	if s1 == s2 {
		t.Error("secrets should be different")
	}
}
