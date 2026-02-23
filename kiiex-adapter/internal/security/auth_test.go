package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSignature(t *testing.T) {
	auth := &Auth{
		APIKey: "testApiKey",
		UserID: 12345,
		Nonce:  "testNonce",
	}

	err := auth.GenerateSignature("testSecret")
	require.NoError(t, err)

	// The signature should be a hex string (64 characters for SHA256)
	assert.Len(t, auth.Signature, 64)
	assert.NotEmpty(t, auth.Signature)

	// Verify deterministic output
	auth2 := &Auth{
		APIKey: "testApiKey",
		UserID: 12345,
		Nonce:  "testNonce",
	}
	err = auth2.GenerateSignature("testSecret")
	require.NoError(t, err)
	assert.Equal(t, auth.Signature, auth2.Signature)
}

func TestGenerateSignatureEmptySecret(t *testing.T) {
	auth := &Auth{
		APIKey: "testApiKey",
		UserID: 12345,
		Nonce:  "testNonce",
	}

	err := auth.GenerateSignature("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret cannot be empty")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		auth    *Auth
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid auth",
			auth: &Auth{
				APIKey:    "testApiKey",
				UserID:    12345,
				Nonce:     "testNonce",
				OmsID:     1,
				AccountID: 100,
			},
			wantErr: false,
		},
		{
			name: "missing APIKey",
			auth: &Auth{
				UserID:    12345,
				Nonce:     "testNonce",
				OmsID:     1,
				AccountID: 100,
			},
			wantErr: true,
			errMsg:  "APIKey is required",
		},
		{
			name: "missing UserID",
			auth: &Auth{
				APIKey:    "testApiKey",
				Nonce:     "testNonce",
				OmsID:     1,
				AccountID: 100,
			},
			wantErr: true,
			errMsg:  "UserID is required",
		},
		{
			name: "missing Nonce",
			auth: &Auth{
				APIKey:    "testApiKey",
				UserID:    12345,
				OmsID:     1,
				AccountID: 100,
			},
			wantErr: true,
			errMsg:  "Nonce is required",
		},
		{
			name: "missing OmsID",
			auth: &Auth{
				APIKey:    "testApiKey",
				UserID:    12345,
				Nonce:     "testNonce",
				AccountID: 100,
			},
			wantErr: true,
			errMsg:  "OmsID is required",
		},
		{
			name: "missing AccountID",
			auth: &Auth{
				APIKey: "testApiKey",
				UserID: 12345,
				Nonce:  "testNonce",
				OmsID:  1,
			},
			wantErr: true,
			errMsg:  "AccountID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
