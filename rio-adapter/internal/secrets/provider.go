package secrets

import (
	"context"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
	pkgsecrets "github.com/Checker-Finance/adapters/rio-adapter/pkg/secrets"
)

// Provider defines a general interface for resolving credentials for a given tenant/client/venue.
type Provider interface {
	Resolve(ctx context.Context, cfg config.Config, clientID, provider string) (pkgsecrets.Credentials, error)
}
