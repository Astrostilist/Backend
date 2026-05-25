package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/golang-jwt/jwt/v4"
)

func AdminAuthMiddleware(cache *memcache.Client, secretTokenAdmin string) func(http.Handler) http.Handler {
	secretTokenAdmin = strings.TrimSpace(secretTokenAdmin)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			if token == "" {
				writeError(w, http.StatusUnauthorized, "admin authorization token is missing")
				return
			}
			//парсим токен, достаем claim
			claims, err := parseJWTClaims(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			// Валидируем claims
			if !VerifyAdminAccessToken(token, secretTokenAdmin, time.Now()) {
				writeError(w, http.StatusUnauthorized, "token invalid")
				return
			}
			// поолучаем jti по ключу и приводим
			jti, ok := claims["jti"].(string)
			if !ok || jti == "" {
				writeError(w, http.StatusUnauthorized, "token missing jti")
				return
			}
			// Проверяем наличие jti в Memcached
			_, err = cache.Get("auth:jti:" + jti)
			if errors.Is(err, memcache.ErrCacheMiss) {
				writeError(w, http.StatusUnauthorized, "token expired or revoked")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "connection error memcached")
				return
			}
			ctx := context.WithValue(r.Context(), jwtClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Парсинг токена с проверкой только claims, подпись проверяется в VerifyAdminAccessToken
func parseJWTClaims(tokenStr string) (jwt.MapClaims, error) {
	parser := jwt.Parser{SkipClaimsValidation: true}
	token, _, err := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims, nil
	}
	return nil, errors.New("invalid claims")
}
