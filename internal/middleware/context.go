package middleware

import (
	"context"
	"net/http"
)

// AuthContext holds the authenticated identity extracted from JWT or API key.
type AuthContext struct {
	UserID         string
	OrganizationID string
	OrgSlug        string
	AuthMethod     string // "jwt" or "apikey"
}

type authContextKey struct{}

// WithAuthContext stores AuthContext in the request context.
func WithAuthContext(ctx context.Context, ac *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, ac)
}

// GetAuthContext retrieves AuthContext from the request context.
// Returns nil if not present.
func GetAuthContext(ctx context.Context) *AuthContext {
	ac, _ := ctx.Value(authContextKey{}).(*AuthContext)
	return ac
}

// MustGetAuthContext retrieves AuthContext or panics.
// Use only in handlers behind the auth middleware.
func MustGetAuthContext(r *http.Request) *AuthContext {
	ac := GetAuthContext(r.Context())
	if ac == nil {
		panic("auth context missing — handler called without auth middleware")
	}
	return ac
}
