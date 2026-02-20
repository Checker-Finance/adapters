package secrets

// Credential represents a normalized secret entry for a client/desk.
type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Venue    string `json:"venue,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
	ClientID string `json:"client_id,omitempty"`
	DeskID   string `json:"desk_id,omitempty"`
}
