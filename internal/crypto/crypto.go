package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

func Encrypt(plaintext string, key []byte) (ciphertext []byte, err error) {
	if len(key) != 32 {
		return nil, errors.New("ключ должен быть длинною 32 байт")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания блочного шифра: %w", err)
	}
	gmc, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания gcm обертки: %w", err)
	}
	nonce := make([]byte, gmc.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("ошибка заполнения буфера случаными числами: %w", err)
	}
	plaintextbyte := []byte(plaintext)
	ciphertext = gmc.Seal(nonce, nonce, plaintextbyte, nil)
	return ciphertext, nil
}
func Decrypt(ciphertext []byte, key []byte) (plaintext string, err error) {
	if len(key) != 32 {
		return "", errors.New("ключ должен быть длинною в 32 байт")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("ошибка создания блочного шифра: %w", err)
	}
	gmc, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("ошибка создания gcm обертки: %w", err)
	}
	nonseSize := gmc.NonceSize()
	if len(ciphertext) < nonseSize {
		return "", errors.New("данные слишком короткие")
	}
	nonse, ciphertextWithTag := ciphertext[:nonseSize], ciphertext[nonseSize:]
	plaintextbyte, err := gmc.Open(nil, nonse, ciphertextWithTag, nil)
	if err != nil {
		return "", fmt.Errorf("ошибка при расшифровки данных: %w", err)
	}
	plaintext = string(plaintextbyte)
	return plaintext, nil
}
