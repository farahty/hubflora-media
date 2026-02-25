package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
)

const apiKeyHeader = "X-Media-API-Key"

// MediaClaims holds the custom claims we extract from JWT tokens.
type MediaClaims struct {
	OrgID   string `json:"orgId"`
	OrgSlug string `json:"orgSlug"`
}

// --------------------------------------------------------------------------
// JWKSCache — thread-safe, TTL-based JWKS cache with key-rotation support.
// --------------------------------------------------------------------------

// JWKSCache fetches and caches a JWKS from a remote endpoint.
type JWKSCache struct {
	mu        sync.RWMutex
	keys      *jose.JSONWebKeySet
	fetchedAt time.Time
	ttl       time.Duration
	jwksURL   string
	client    *http.Client
}

// NewJWKSCache creates a new cache that fetches keys from jwksURL and
// considers them fresh for ttl.
func NewJWKSCache(jwksURL string, ttl time.Duration) *JWKSCache {
	return &JWKSCache{
		jwksURL: jwksURL,
		ttl:     ttl,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetKeys returns the cached JWKS, refreshing it when the TTL has expired.
// If the fetch fails and a stale cache exists, the stale cache is returned.
func (c *JWKSCache) GetKeys(ctx context.Context) (*jose.JSONWebKeySet, error) {
	c.mu.RLock()
	if c.keys != nil && time.Since(c.fetchedAt) < c.ttl {
		ks := c.keys
		c.mu.RUnlock()
		return ks, nil
	}
	// Need refresh — capture stale copy for fallback.
	stale := c.keys
	c.mu.RUnlock()

	ks, err := c.fetchJWKS(ctx)
	if err != nil {
		if stale != nil {
			slog.Warn("jwks fetch failed, using stale cache", "error", err)
			return stale, nil
		}
		return nil, fmt.Errorf("jwks fetch failed and no stale cache: %w", err)
	}

	c.mu.Lock()
	c.keys = ks
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	return ks, nil
}

// RefreshForKID forces a re-fetch if the given kid is not present in the
// current cache (handles key rotation). If the kid is already cached the
// existing key set is returned immediately.
func (c *JWKSCache) RefreshForKID(ctx context.Context, kid string) (*jose.JSONWebKeySet, error) {
	c.mu.RLock()
	if c.keys != nil {
		if matches := c.keys.Key(kid); len(matches) > 0 {
			ks := c.keys
			c.mu.RUnlock()
			return ks, nil
		}
	}
	stale := c.keys
	c.mu.RUnlock()

	slog.Info("jwks cache miss for kid, re-fetching", "kid", kid)

	ks, err := c.fetchJWKS(ctx)
	if err != nil {
		if stale != nil {
			slog.Warn("jwks re-fetch failed, using stale cache", "error", err)
			return stale, nil
		}
		return nil, fmt.Errorf("jwks re-fetch failed and no stale cache: %w", err)
	}

	c.mu.Lock()
	c.keys = ks
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	return ks, nil
}

// fetchJWKS performs the actual HTTP request to the JWKS endpoint.
func (c *JWKSCache) fetchJWKS(ctx context.Context) (*jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build jwks request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jwks HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("jwks endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var ks jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&ks); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	return &ks, nil
}

// --------------------------------------------------------------------------
// DualAuth middleware — tries JWT first, falls back to API key.
// --------------------------------------------------------------------------

// supportedAlgorithms lists the JWS algorithms accepted for token verification.
var supportedAlgorithms = []jose.SignatureAlgorithm{
	jose.EdDSA,
	jose.RS256,
	jose.ES256,
	jose.ES384,
	jose.ES512,
}

// DualAuth returns middleware that authenticates requests using either:
//  1. JWT (Authorization: Bearer <token>) validated via JWKS, or
//  2. API key (X-Media-API-Key header) for service-to-service calls.
//
// On success the request context is enriched with an AuthContext.
func DualAuth(apiKey string, jwksCache *JWKSCache, betterAuthURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// --- attempt JWT ---
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				ac, err := validateJWT(r.Context(), token, jwksCache, betterAuthURL)
				if err != nil {
					slog.Warn("jwt validation failed", "error", err)
					http.Error(w, `{"error":"invalid or expired JWT"}`, http.StatusUnauthorized)
					return
				}
				ctx := WithAuthContext(r.Context(), ac)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// --- attempt API key ---
			if key := r.Header.Get(apiKeyHeader); key != "" {
				if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
					http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
					return
				}
				userID := r.Header.Get("X-Media-User-Id")
				if userID == "" {
					http.Error(w, `{"error":"X-Media-User-Id header is required"}`, http.StatusBadRequest)
					return
				}
				ac := &AuthContext{
					UserID:         userID,
					OrganizationID: r.Header.Get("X-Media-Org-Id"),
					OrgSlug:        r.Header.Get("X-Media-Org-Slug"),
					AuthMethod:     "apikey",
				}
				ctx := WithAuthContext(r.Context(), ac)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// --- neither provided ---
			http.Error(w, `{"error":"missing authentication"}`, http.StatusUnauthorized)
		})
	}
}

// validateJWT parses and verifies a JWT token using the cached JWKS.
func validateJWT(ctx context.Context, raw string, cache *JWKSCache, betterAuthURL string) (*AuthContext, error) {
	// Parse the compact-serialized JWS.
	tok, err := josejwt.ParseSigned(raw, supportedAlgorithms)
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}

	// Identify the signing key by kid.
	if len(tok.Headers) == 0 {
		return nil, fmt.Errorf("jwt has no headers")
	}
	kid := tok.Headers[0].KeyID

	// Fetch JWKS — try cache first, refresh on unknown kid.
	var ks *jose.JSONWebKeySet
	if kid != "" {
		ks, err = cache.RefreshForKID(ctx, kid)
	} else {
		ks, err = cache.GetKeys(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("get jwks: %w", err)
	}

	// Find matching key(s).
	var matchingKeys []jose.JSONWebKey
	if kid != "" {
		matchingKeys = ks.Key(kid)
	} else {
		// No kid — try all keys.
		matchingKeys = ks.Keys
	}
	if len(matchingKeys) == 0 {
		return nil, fmt.Errorf("no matching key found for kid=%q", kid)
	}

	// Try each matching key until one verifies.
	var claims josejwt.Claims
	var custom MediaClaims
	var verifyErr error

	for _, key := range matchingKeys {
		verifyErr = tok.Claims(key.Key, &claims, &custom)
		if verifyErr == nil {
			break
		}
	}
	if verifyErr != nil {
		return nil, fmt.Errorf("verify jwt signature: %w", verifyErr)
	}

	// Validate standard claims.
	expected := josejwt.Expected{
		Issuer:      betterAuthURL,
		AnyAudience: josejwt.Audience{betterAuthURL},
		Time:        time.Now(),
	}
	if err := claims.Validate(expected); err != nil {
		return nil, fmt.Errorf("validate jwt claims: %w", err)
	}

	return &AuthContext{
		UserID:         claims.Subject,
		OrganizationID: custom.OrgID,
		OrgSlug:        custom.OrgSlug,
		AuthMethod:     "jwt",
	}, nil
}

// --------------------------------------------------------------------------
// Legacy API-key-only middleware (backward compatibility).
// --------------------------------------------------------------------------

// APIKeyAuth validates requests against a shared API key.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(apiKeyHeader)
			if key == "" {
				http.Error(w, `{"error":"missing API key"}`, http.StatusUnauthorized)
				return
			}
			if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
