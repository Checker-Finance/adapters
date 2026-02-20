package auth

// Credentials represents username/password needed to log in to Braza.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenBundle represents access + refresh token data.
type TokenBundle struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Exp          int64  `json:"exp"` // Unix timestamp
}
