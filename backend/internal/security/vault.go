package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

type Vault struct{ aead cipher.AEAD }

func NewVault(secret string) (*Vault, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("encryption key must contain at least 32 characters")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func (v *Vault) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := v.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (v *Vault) Decrypt(encoded string) (string, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(sealed) < v.aead.NonceSize() {
		return "", fmt.Errorf("invalid encrypted value")
	}
	nonce := sealed[:v.aead.NonceSize()]
	plaintext, err := v.aead.Open(nil, nonce, sealed[v.aead.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt encrypted value: %w", err)
	}
	return string(plaintext), nil
}
