package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const meshKeyPrefix = "mesh_live_"

func GenerateMeshKey(pepper string) (plaintext, hash, displayPrefix string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}

	body := base64.RawURLEncoding.EncodeToString(buf)
	plaintext = meshKeyPrefix + body
	hash = HashMeshKey(pepper, plaintext)
	displayPrefix = meshKeyPrefix + body[:6] + "..."

	return plaintext, hash, displayPrefix, nil
}

func HashMeshKey(pepper, plaintext string) string {
	sum := HashMeshKeyRaw(pepper, plaintext)
	return hex.EncodeToString(sum[:])
}

func HashMeshKeyRaw(pepper, plaintext string) [32]byte {
	return sha256.Sum256([]byte(pepper + plaintext))
}
