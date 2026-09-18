package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

func newGCM(masterKeyHex string) (cipher.AEAD, error) {
	key, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("master key must decode to 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func EncryptWithMasterKey(masterKeyHex, plaintext string) (ciphertext, nonce []byte, err error) {
	gcm, err := newGCM(masterKeyHex)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}

func DecryptWithMasterKey(masterKeyHex string, ciphertext, nonce []byte) (string, error) {
	gcm, err := newGCM(masterKeyHex)
	if err != nil {
		return "", err
	}
	return DecryptWithCipher(gcm, ciphertext, nonce)
}

func NewMasterCipher(masterKeyHex string) (cipher.AEAD, error) {
	return newGCM(masterKeyHex)
}

func DecryptWithCipher(gcm cipher.AEAD, ciphertext, nonce []byte) (string, error) {
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
