package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
)

// UnsubscribePath is where the links to stop emails go: <PUBLIC_URL>/unsubscribe/<destination id>?token=…
const UnsubscribePath = "/unsubscribe/"

// Unsubscriber signs the links that stop the emails to an address (one
// destination) and checks them: a key derived from SECRET_KEY or secret.key,
// so the links of sent emails keep working across restarts. The zero value
// signs nothing, and its emails carry no link.
type Unsubscriber struct{ key []byte }

func NewUnsubscriber(secretKey string) Unsubscriber {
	if secretKey == "" {
		return Unsubscriber{}
	}

	sum := sha256.Sum256([]byte("unsubscribe\x00" + secretKey))

	return Unsubscriber{key: sum[:]}
}

func (unsubscriber Unsubscriber) token(destinationID string) []byte {
	mac := hmac.New(sha256.New, unsubscriber.key)
	mac.Write([]byte(destinationID))

	return mac.Sum(nil)[:16]
}

// Valid says whether token is the link's of the destination.
func (unsubscriber Unsubscriber) Valid(destinationID, token string) bool {
	given, err := hex.DecodeString(token)

	return unsubscriber.key != nil && err == nil && hmac.Equal(given, unsubscriber.token(destinationID))
}

// URL is the link that stops the emails to the destination; empty without a
// key or a public address to put in it.
func (unsubscriber Unsubscriber) URL(publicURL, destinationID string) string {
	if unsubscriber.key == nil || publicURL == "" || destinationID == "" {
		return ""
	}

	return publicURL + UnsubscribePath + url.PathEscape(destinationID) + "?" + url.Values{"token": {hex.EncodeToString(unsubscriber.token(destinationID))}}.Encode()
}
