package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Auth represents the authentication credentials from AWS Secrets
type Auth struct {
	APIKey    string `json:"APIKey"`
	Signature string `json:"Signature"`
	UserID    int    `json:"UserId"`
	Nonce     string `json:"Nonce"`
	OmsID     int    `json:"OMSId"`
	AccountID int    `json:"AccountId"`
	Username  string `json:"Username"`
	Secret    string `json:"Secret,omitempty"` // Not serialized, used for signature generation
}

// GenerateSignature generates the HMAC-SHA256 signature
// The signature is: HMAC-SHA256(secret, Nonce + UserId + APIKey)
func (a *Auth) GenerateSignature(secret string) error {
	if secret == "" {
		return fmt.Errorf("secret cannot be empty")
	}

	data := fmt.Sprintf("%s%d%s", a.Nonce, a.UserID, a.APIKey)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	a.Signature = hex.EncodeToString(h.Sum(nil))

	return nil
}

// Validate checks if the auth credentials are valid
func (a *Auth) Validate() error {
	if a.APIKey == "" {
		return fmt.Errorf("APIKey is required")
	}
	if a.UserID == 0 {
		return fmt.Errorf("UserID is required")
	}
	if a.Nonce == "" {
		return fmt.Errorf("nonce is required")
	}
	if a.OmsID == 0 {
		return fmt.Errorf("OmsID is required")
	}
	if a.AccountID == 0 {
		return fmt.Errorf("AccountID is required")
	}
	return nil
}
