package dodo

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// VerifySignature checks a Dodo webhook against the Standard Webhooks spec:
// HMAC-SHA256 over "<webhook-id>.<webhook-timestamp>.<raw-body>", where the
// signing key is base64-decoded after stripping the "whsec_" prefix.
// Dodo sends one or more space-separated "v1,<base64-sig>" values.
func VerifySignature(secret, rawBody, webhookID, webhookTimestamp, webhookSignature string) bool {
	if secret == "" {
		return false
	}

	keyBase64 := strings.TrimPrefix(secret, "whsec_")
	keyBytes, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return false
	}

	message := webhookID + "." + webhookTimestamp + "." + rawBody
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, part := range strings.Fields(webhookSignature) {
		pieces := strings.Split(part, ",")
		sig := pieces[len(pieces)-1]
		if sig == "" {
			continue
		}
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return true
		}
	}
	return false
}
