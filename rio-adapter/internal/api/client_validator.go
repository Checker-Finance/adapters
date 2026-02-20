package api

import (
	"context"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
)

// ResolverValidator implements ClientValidator by attempting to resolve
// the client's config via ConfigResolver. If resolution succeeds (cache
// hit or AWS Secrets Manager lookup), the client is considered known.
type ResolverValidator struct {
	resolver rio.ConfigResolver
}

// NewResolverValidator creates a ClientValidator backed by a ConfigResolver.
func NewResolverValidator(resolver rio.ConfigResolver) *ResolverValidator {
	return &ResolverValidator{resolver: resolver}
}

// IsKnownClient returns true if the client has a valid config in AWS Secrets Manager.
func (v *ResolverValidator) IsKnownClient(ctx context.Context, clientID string) bool {
	_, err := v.resolver.Resolve(ctx, clientID)
	return err == nil
}
