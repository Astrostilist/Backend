package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "Шифрование даты",
			plaintext: "01.01.2001",
		},
		{
			name:      "Пустая строка",
			plaintext: "",
		},
		{
			name:      "Шифрование даты (длинная строка)",
			plaintext: "пятое сентября дветысячи второго года",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := Encrypt(tt.plaintext, key)
			if err != nil {
				t.Fatalf("Encrypt() : %v", err)
			}
			decrypt, err := Decrypt(ciphertext, key)
			if err != nil {
				t.Fatalf("Decrypt() : %v", err)
			}
			if decrypt != tt.plaintext {
				t.Errorf("Encrypt → Decrypt не вернул исходную строку\nПолучено: %q\nОжидалось: %q",
					decrypt, tt.plaintext)
			}
			// Проверка длины
			if len(decrypt) != len(tt.plaintext) {
				t.Errorf("Длина не совпадает: получили %d, ожидали %d",
					len(decrypt), len(tt.plaintext))
			}
		})
	}
}
func TestRandomNonce(t *testing.T) {
	plaintext := "одинаковое сообщение"
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	// Шифруем 3 раза
	ciphertexts := make([][]byte, 3)
	for i := 0; i < 3; i++ {
		ct, err := Encrypt(plaintext, key)
		if err != nil {
			t.Fatalf("Encrypt #%d failed: %v", i, err)
		}
		ciphertexts[i] = ct
	}
	// Проверяем, что все шифротексты разные
	for i := 0; i < len(ciphertexts); i++ {
		for j := i + 1; j < len(ciphertexts); j++ {
			if bytes.Equal(ciphertexts[i], ciphertexts[j]) {
				t.Errorf("Ciphertext %d and %d are equal, but should be different due to random nonce", i, j)
			}
		}
	}
}

func TestBadEncryptDecrypt(t *testing.T) {
	validKey := make([]byte, 32)
	if _, err := rand.Read(validKey); err != nil {
		t.Fatal(err)
	}
	plaintext := "01.01.2001"
	ciphertext, err := Encrypt(plaintext, validKey)
	if err != nil {
		t.Fatalf("Encrypt() не должен выдавать ошибку: %v", err)
	}
	t.Run("Расшифровка с другим ключом", func(t *testing.T) {
		anotherKey := make([]byte, 32)
		if _, err := rand.Read(anotherKey); err != nil {
			t.Fatal(err)
		}
		_, err := Decrypt(ciphertext, anotherKey)
		if err == nil {
			t.Error("Decrypt() с неверным ключом должен вернуть ошибку, но вернул nil")
		}
	})
	t.Run("Расшифровка с ключом неверной длины", func(t *testing.T) {
		invalidKey := make([]byte, 16)
		_, err := Decrypt(ciphertext, invalidKey)
		if err == nil {
			t.Error("Decrypt() с ключом 16 байт должен вернуть ошибку, но вернул nil")
		}
	})

}
