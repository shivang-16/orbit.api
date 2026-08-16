package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const keyPrefix = "sk-orbit-"

func generateSecret() (secret string, preview string, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", "", fmt.Errorf("generate key: %w", err)
	}

	secret = keyPrefix + hex.EncodeToString(raw)
	hash = HashSecret(secret)
	preview = secret[:len(keyPrefix)+3] + " ... " + secret[len(secret)-3:]
	return secret, preview, hash, nil
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
