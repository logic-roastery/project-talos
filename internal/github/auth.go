package github

import (
	"crypto/rsa"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ParsePrivateKey reads a PEM-encoded RSA private key from a string or file path.
func ParsePrivateKey(keyOrPath string) (*rsa.PrivateKey, error) {
	pemData := keyOrPath
	if strings.HasPrefix(keyOrPath, "/") || strings.HasPrefix(keyOrPath, "./") || strings.HasPrefix(keyOrPath, "../") {
		data, err := os.ReadFile(keyOrPath)
		if err != nil {
			return nil, fmt.Errorf("read private key file: %w", err)
		}
		pemData = string(data)
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(pemData))
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}
	return key, nil
}

// GenerateJWT creates a signed JWT for GitHub App authentication.
// The token is valid for 10 minutes.
func GenerateJWT(appID int64, privateKey *rsa.PrivateKey) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    fmt.Sprintf("%d", appID),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}
