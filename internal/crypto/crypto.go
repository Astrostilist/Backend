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
		return nil, errors.New("the key must be 32 bytes long")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("block cipher creation error: %w", err)
	}
	gmc, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("error creating gcm wrapper: %w", err)
	}
	nonce := make([]byte, gmc.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("error filling buffer with random numbers: %w", err)
	}
	plaintextbyte := []byte(plaintext)
	ciphertext = gmc.Seal(nonce, nonce, plaintextbyte, nil)
	return ciphertext, nil
}
func Decrypt(ciphertext []byte, key []byte) (plaintext string, err error) {
	if len(key) != 32 {
		return "", errors.New("the key must be 32 bytes long")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("block cipher creation error: %w", err)
	}
	gmc, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creating gcm wrapper: %w", err)
	}
	nonseSize := gmc.NonceSize()
	if len(ciphertext) < nonseSize {
		return "", errors.New("data is too short")
	}
	nonse, ciphertextWithTag := ciphertext[:nonseSize], ciphertext[nonseSize:]
	plaintextbyte, err := gmc.Open(nil, nonse, ciphertextWithTag, nil)
	if err != nil {
		return "", fmt.Errorf("error while decrypting data: %w", err)
	}
	plaintext = string(plaintextbyte)
	return plaintext, nil
}
