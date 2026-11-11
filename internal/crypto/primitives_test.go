package crypto

import (
	"bytes"
	"testing"
)

func TestX25519Exchange(t *testing.T) {
	// Алиса
	alicePub, alicePriv, err := GenerateX25519Keypair()
	if err != nil {
		t.Fatal(err)
	}
	// Боб
	bobPub, bobPriv, err := GenerateX25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	// Вычисление секрета
	aliceSecret, err := ComputeSharedSecret(alicePriv, bobPub)
	if err != nil {
		t.Fatal(err)
	}
	bobSecret, err := ComputeSharedSecret(bobPriv, alicePub)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(aliceSecret, bobSecret) {
		t.Fatal("shared secrets do not match")
	}
}

func TestHKDFDerivation(t *testing.T) {
	secret := bytes.Repeat([]byte{0xAA}, 32)
	salt := bytes.Repeat([]byte{0xBB}, 32)
	info := []byte("p2p-node-v1")

	// Алиса — клиент (инициатор)
	aSend, aRecv, err := DeriveKeys(secret, salt, info, true)
	if err != nil {
		t.Fatal(err)
	}
	// Боб — сервер (ответчик)
	bSend, bRecv, err := DeriveKeys(secret, salt, info, false)
	if err != nil {
		t.Fatal(err)
	}

	// Алиса шлёт aSend, Боб принимает bRecv — должны совпадать
	if !bytes.Equal(aSend, bRecv) {
		t.Fatal("send/recv key mismatch for Alice->Bob")
	}
	// Боб шлёт bSend, Алиса принимает aRecv — должны совпадать
	if !bytes.Equal(aRecv, bSend) {
		t.Fatal("recv/send key mismatch for Bob->Alice")
	}

	// Исходящий и входящий ключи должны различаться (направления разные)
	if bytes.Equal(aSend, aRecv) {
		t.Fatal("send and recv keys must be different")
	}
}

func TestAEAD(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, KeySize)
	nonce := bytes.Repeat([]byte{0x02}, NonceSize)
	aad := []byte("handshake-transcript-hash")
	plaintext := []byte("secret DHT record payload")

	ciphertext, err := Encrypt(key, nonce, plaintext, aad)
	if err != nil {
		t.Fatal(err)
	}

	// Успешная расшифровка
	decrypted, err := Decrypt(key, nonce, ciphertext, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Fatal("plaintext mismatch after decryption")
	}

	// Атака 1: изменение шифротекста
	ciphertext[0] ^= 0xFF
	if _, err := Decrypt(key, nonce, ciphertext, aad); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	ciphertext[0] ^= 0xFF // возвращаем назад

	// Атака 2: изменение AAD (контекста)
	if _, err := Decrypt(key, nonce, ciphertext, []byte("wrong-transcript")); err == nil {
		t.Fatal("tampered AAD accepted")
	}

	// Атака 3: неверный nonce
	badNonce := bytes.Repeat([]byte{0x03}, NonceSize)
	if _, err := Decrypt(key, badNonce, ciphertext, aad); err == nil {
		t.Fatal("wrong nonce accepted")
	}
}
