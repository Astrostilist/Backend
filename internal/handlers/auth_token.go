package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	adminTokenTTL        = 24 * time.Hour
	authTokenPartsNumber = 2
)

var ErrInvalidAuthToken = errors.New("invalid auth token")

type adminTokenClaims struct {
	Subject   string `json:"sub"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
}

func GenerateAdminAccessToken(userID, email, role, secret string, now time.Time) (string, error) {
	secret = strings.TrimSpace(secret)
	valid := userID != "" && email != "" && role != "" && secret != ""
	if !valid {
		return "", ErrInvalidAuthToken
	}

	claims := adminTokenClaims{
		Subject:   userID,
		Email:     email,
		Role:      role,
		ExpiresAt: now.Add(adminTokenTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal admin token claims: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := signAdminToken(encodedPayload, secret)
	return encodedPayload + "." + signature, nil
}

func VerifyAdminAccessToken(token, secret string, now time.Time) bool {
	parts := strings.Split(token, ".")
	validParts := len(parts) == authTokenPartsNumber && strings.TrimSpace(secret) != ""
	if !validParts {
		return false
	}

	expectedSignature := signAdminToken(parts[0], secret)
	if !hmac.Equal([]byte(parts[1]), []byte(expectedSignature)) {
		return false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	var claims adminTokenClaims

	if err = json.Unmarshal(payload, &claims); err != nil {
		return false
	}

	return claims.Subject != "" && claims.Role == "super_admin" && claims.ExpiresAt > now.Unix()
}

func signAdminToken(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
