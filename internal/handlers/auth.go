package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"astroapi/internal/admin"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

type contextKey string

const jwtClaimsKey contextKey = "jwt_claims"

type AuthHandler struct {
	repository  admin.Repository
	secretToken string
	now         func() time.Time
	cache       *memcache.Client
}

type adminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type adminLoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func NewAuthHandler(repository admin.Repository, tokenSecret string, cache *memcache.Client) *AuthHandler {
	return &AuthHandler{
		repository:  repository,
		secretToken: strings.TrimSpace(tokenSecret),
		now:         time.Now,
		cache:       cache,
	}
}
func saveJTItoMemcached(cache *memcache.Client, jti, userID string, expSeconds int32) error {
	key := fmt.Sprintf("auth:jti:%s", jti)
	err := cache.Set(&memcache.Item{
		Key:        key,
		Value:      []byte(userID),
		Expiration: expSeconds,
	})
	if err != nil {
		return err
	}
	return nil
}
func addUserJTI(cache *memcache.Client, userID, jti string, ttl int32) error {
	key := "user_tokens:" + userID
	for {
		//пытаемся прочитать jti в кеше
		item, err := cache.Get(key)
		var jtis []string
		if err == nil {
			// Ключ существует — парсим JSON-массив в срез jtis
			json.Unmarshal(item.Value, &jtis)
		} else if err != memcache.ErrCacheMiss {
			// Реальная ошибка (сеть, таймаут) — сразу возвращаем
			return err
		}
		// Если ErrCacheMiss, то jtis остаётся пустым срезом — это нормально
		// Добавляем новый jti
		jtis = append(jtis, jti)
		newValue, _ := json.Marshal(jtis) // Сериализуем обновлённый список

		if err == nil {
			// Ключ существовал — пробуем обновить через CAS
			err = cache.CompareAndSwap(&memcache.Item{
				Key:   key,
				Value: newValue,
				CasID: item.CasID,
			})
			if err == memcache.ErrCASConflict {
				// Кто-то другой уже изменил ключ — начинаем заново
				continue
			}
			return err // Либо успех (err == nil), либо неожиданная ошибка
		}

		// Ключ не существовал — просто создаём через Set
		return cache.Set(&memcache.Item{
			Key:        key,
			Value:      newValue,
			Expiration: ttl,
		})
	}
}
func revokeAllUserTokens(cache *memcache.Client, userID string) error {
	key := "user_tokens:" + userID
	item, err := cache.Get(key)
	if err == memcache.ErrCacheMiss {
		return nil
	}
	if err != nil {
		return err
	}
	var jtis []string
	if err := json.Unmarshal(item.Value, &jtis); err != nil {
		return err
	}
	// Удаляем каждый jti
	for _, j := range jtis {
		cache.Delete("auth:jti:" + j) // ошибки игнорируем, т.к. ключ может уже истечь
	}
	// Удаляем сам список
	return cache.Delete(key)
}

// перезаписывает массив jti пользователя без указанного jti
func removeJTIFromUserList(cache *memcache.Client, userID, jti string) error {
	key := "user_tokens:" + userID
	for {
		item, err := cache.Get(key)
		if err == memcache.ErrCacheMiss {
			return nil // ключа нет — ок
		}
		if err != nil {
			return err
		}

		var jtis []string
		json.Unmarshal(item.Value, &jtis)

		filtered := jtis[:0]
		for _, j := range jtis {
			if j != jti {
				filtered = append(filtered, j)
			}
		}

		newValue, _ := json.Marshal(filtered)
		err = cache.CompareAndSwap(&memcache.Item{
			Key:   key,
			Value: newValue,
			CasID: item.CasID,
		})
		if err == memcache.ErrCASConflict {
			continue
		}
		return nil
	}
}
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Проверка и вертификация пользователя
	if h.repository == nil {
		writeError(w, http.StatusInternalServerError, "auth repository is not configured")
		return
	}
	if h.secretToken == "" {
		writeError(w, http.StatusInternalServerError, "admin token secret is not configured")
		return
	}

	var payload adminLoginRequest

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid login request")
		return
	}

	user, err := h.repository.VerifyCredentials(r.Context(), payload.Email, payload.Password)
	if err != nil {
		if errors.Is(err, admin.ErrInvalidCredential) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to verify admin credentials")
		return
	}

	// Инвалидируем все старые записи в токены пользователя
	err = revokeAllUserTokens(h.cache, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cache invalidation error")
	}
	// Гененрируем новый токен
	jti := uuid.New().String()
	accessToken, err := GenerateAdminAccessToken(user.ID, user.Email, user.Role, h.secretToken, jti, h.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate admin access token")
		return
	}
	// Сохраняем jti в memcached auth:jti:{jti} {user_id} EX {exp}
	err = saveJTItoMemcached(h.cache, jti, user.ID, int32(adminTokenTTL.Seconds()))
	if err != nil {
		// скорее всего нужно логировать но не прерывать, так как токен может быть уже выдан

	}
	// Добавляем jti в список активных токенов пользователя ключ user_tokens:{userID} значение JSON-массив всех активных `jti`
	err = addUserJTI(h.cache, user.ID, jti, int32(adminTokenTTL.Seconds()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error adding jti to the user's list of active tokens")
		return
	}

	writeJSON(w, http.StatusOK, Response{
		Message: "Admin authenticated successfully",
		Data: adminLoginResponse{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			ExpiresIn:   int64(adminTokenTTL.Seconds()),
		},
	})
}
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {

	claims, ok := r.Context().Value(jwtClaimsKey).(jwt.MapClaims)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing token claims")
		return
	}
	jti := claims["jti"].(string)
	userID := claims["sub"].(string)

	// Удаляем ключ auth:jti:{jti}
	if err := h.cache.Delete("auth:jti:" + jti); err != nil && err != memcache.ErrCacheMiss {
		writeError(w, http.StatusInternalServerError, "failed to delete jti")
		return
	}
	// Удаляем jti из списка активных токенов пользователя
	_ = removeJTIFromUserList(h.cache, userID, jti)

	w.WriteHeader(http.StatusNoContent)
}
