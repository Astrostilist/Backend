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
	adminTokenTTL       = 24 * time.Hour
	jwtTokenPartsNumber = 3
	jwtAlgorithmHS256   = "HS256"
	jwtTokenType        = "JWT"
)

var ErrInvalidAuthToken = errors.New("invalid auth token")

type adminTokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type adminTokenClaims struct {
	Subject   string `json:"sub"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
}

func GenerateAdminAccessToken(userID, email, role, secret string, now time.Time) (string, error) {
	secret = strings.TrimSpace(secret)
	valid := userID != "" && email != "" && role != "" && secret != ""
	if !valid {
		return "", ErrInvalidAuthToken
	}

	header := adminTokenHeader{
		Algorithm: jwtAlgorithmHS256,
		Type:      jwtTokenType,
	}
	claims := adminTokenClaims{
		Subject:   userID,
		Email:     email,
		Role:      role,
		ExpiresAt: now.Add(adminTokenTTL).Unix(),
		IssuedAt:  now.Unix(),
	}

	encodedHeader, err := encodeJWTPart(header)
	if err != nil {
		return "", err
	}
	encodedPayload, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}

	signingInput := encodedHeader + "." + encodedPayload
	signature := signAdminToken(signingInput, secret)
	return signingInput + "." + signature, nil
}

func VerifyAdminAccessToken(token, secret string, now time.Time) bool {
	parts := strings.Split(strings.TrimSpace(token), ".")
	validParts := len(parts) == jwtTokenPartsNumber && strings.TrimSpace(secret) != ""
	if !validParts {
		return false
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSignature := signAdminToken(signingInput, secret)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSignature)) {
		return false
	}

	var header adminTokenHeader
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return false
	}
	if header.Algorithm != jwtAlgorithmHS256 || header.Type != jwtTokenType {
		return false
	}

	var claims adminTokenClaims
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return false
	}

	return claims.Subject != "" &&
		claims.Email != "" &&
		claims.Role == "super_admin" &&
		claims.ExpiresAt > now.Unix()
}

func encodeJWTPart(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal admin jwt part: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeJWTPart(encoded string, destination any) error {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, destination)
}

func signAdminToken(signingInput, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
