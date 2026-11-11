// Package crypto предоставляет криптографические примитивы для AKE.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	KeySize   = 32                         // 32 байта для X25519 и ChaCha20
	NonceSize = chacha20poly1305.NonceSize // 12 байт
	TagSize   = chacha20poly1305.Overhead  // 16 байт
)

// GenerateX25519Keypair генерирует эфемерную пару ключей X25519.
func GenerateX25519Keypair() (pub, priv []byte, err error) {
	priv = make([]byte, curve25519.ScalarSize)
	if _, err := io.ReadFull(rand.Reader, priv); err != nil {
		return nil, nil, fmt.Errorf("generate private key: %w", err)
	}
	// Очистка младших битов (clamping) делается автоматически в curve25519.X25519
	pub, err = curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("derive public key: %w", err)
	}
	return pub, priv, nil
}

// ComputeSharedSecret вычисляет общий секрет (ECDH).
func ComputeSharedSecret(priv, peerPub []byte) ([]byte, error) {
	secret, err := curve25519.X25519(priv, peerPub)
	if err != nil {
		return nil, fmt.Errorf("compute shared secret: %w", err)
	}
	return secret, nil
}

// DeriveKeys деривирует ключи шифрования из общего секрета.
// Параметр isClient указывает роль стороны:
// -клиент (инициатор) получает (sendKey = key_c2s, recvKey = key_s2c)
// -сервер (ответчик) получает (sendKey = key_s2c, recvKey = key_c2s)
// Это гарантирует, что ключ отправки одной стороны совпадает
// с ключом приёма другой.
func DeriveKeys(sharedSecret, salt, info []byte, isClient bool) (sendKey, recvKey []byte, err error) {
	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, info)

	// Растягиваем на 64 байта: key_c2s || key_s2c
	keyMaterial := make([]byte, 64)
	if _, err := io.ReadFull(hkdfReader, keyMaterial); err != nil {
		return nil, nil, fmt.Errorf("hkdf read: %w", err)
	}

	keyC2S := keyMaterial[:32]
	keyS2C := keyMaterial[32:]

	if isClient {
		return keyC2S, keyS2C, nil
	}
	return keyS2C, keyC2S, nil
}

// Encrypt шифрует plaintext с помощью ChaCha20-Poly1305.
// aad (Additional Authenticated Data) используется для привязки шифротекста
// к контексту (например, хэшу транскрипта рукопожатия).
func Encrypt(key, nonce, plaintext, aad []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, errors.New("invalid key size")
	}
	if len(nonce) != NonceSize {
		return nil, errors.New("invalid nonce size")
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("create aead: %w", err)
	}

	return aead.Seal(nil, nonce, plaintext, aad), nil
}

// Decrypt расшифровывает ciphertext и проверяет MAC.
func Decrypt(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, errors.New("invalid key size")
	}
	if len(nonce) != NonceSize {
		return nil, errors.New("invalid nonce size")
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("create aead: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	return plaintext, nil
}
