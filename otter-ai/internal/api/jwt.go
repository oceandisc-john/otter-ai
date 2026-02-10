package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT configuration constants
const (
	JWTExpirationTime = 24 * time.Hour // Token expires after 24 hours
	JWTIssuer         = "otter-ai"
)

// Claims represents the JWT claims
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// JWTManager handles JWT token operations
type JWTManager struct {
	secretKey []byte
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(secret string) (*JWTManager, error) {
	if secret == "" {
		// Generate a random secret if none provided
		secret = generateRandomSecret()
	}

	return &JWTManager{
		secretKey: []byte(secret),
	}, nil
}

// GenerateToken generates a new JWT token for a user
func (m *JWTManager) GenerateToken(userID string) (string, error) {
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(JWTExpirationTime)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    JWTIssuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secretKey)
}

// ValidateToken validates a JWT token and returns the claims
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}

// generateRandomSecret generates a random secret key
func generateRandomSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based secret (not cryptographically secure)
		return fmt.Sprintf("otter-ai-secret-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
