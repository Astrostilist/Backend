package handlers

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const adminTokenTTL = 24 * time.Hour

var ErrInvalidAuthToken = errors.New("invalid auth token")

type adminTokenClaims struct {
	Subject   string `json:"sub"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"exp"`
	JTI       string `json:"jti"`
}

func (c adminTokenClaims) Valid() error {
	if time.Now().Unix() > c.ExpiresAt {
		return ErrInvalidAuthToken
	}
	return nil
}

func GenerateAdminAccessToken(userID, email, role, secret, jti string, now time.Time) (string, error) {
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
		JTI:       jti,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

func VerifyAdminAccessToken(tokenStr, secret string, now time.Time) bool {
	if strings.TrimSpace(secret) == "" {
		return false
	}

	var claims adminTokenClaims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidAuthToken
		}
		return []byte(secret), nil
	})

	if err != nil || !token.Valid {
		return false
	}

	return claims.Subject != "" && claims.Role == "super_admin" && claims.ExpiresAt > now.Unix()
}
