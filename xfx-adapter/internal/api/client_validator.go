package api

import (
	"context"

	"github.com/Checker-Finance/adapters/xfx-adapter/internal/xfx"
)

// ResolverValidator implements ClientValidator by attempting to resolve
// the client's config via ConfigResolver. Resolution succeeds only if
// the client has a valid secret in AWS Secrets Manager.
type ResolverValidator struct {
	resolver xfx.ConfigResolver
}

// NewResolverValidator creates a ClientValidator backed by a ConfigResolver.
func NewResolverValidator(resolver xfx.ConfigResolver) *ResolverValidator {
	return &ResolverValidator{resolver: resolver}
}

// IsKnownClient returns true if the client has valid config in AWS Secrets Manager.
func (v *ResolverValidator) IsKnownClient(ctx context.Context, clientID string) bool {
	_, err := v.resolver.Resolve(ctx, clientID)
	return err == nil
}
