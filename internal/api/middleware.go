package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/KB-perByte/hiveshare/internal/models"
	"github.com/KB-perByte/hiveshare/internal/store"
)

type contextKey string

const ctxUser contextKey = "user"

const (
	authCacheTTL     = 60 * time.Second
	authCacheMaxSize = 256
)

type authCacheEntry struct {
	user      *models.User
	expiresAt time.Time
}

// authCache is a small in-process TTL cache for API-key hash → User lookups.
// It bounds the number of DB round-trips on high-traffic endpoints. Entries
// expire after authCacheTTL and are evicted LRU once authCacheMaxSize is reached.
type authCache struct {
	mu      sync.Mutex
	entries map[string]*authCacheEntry
	order   []string // oldest at front
}

func newAuthCache() *authCache {
	return &authCache{entries: make(map[string]*authCacheEntry)}
}

func (c *authCache) get(keyHash string) (*models.User, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[keyHash]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		c.removeLocked(keyHash)
		return nil, false
	}
	// move to most-recently-used (end)
	c.touchLocked(keyHash)
	return e.user, true
}

func (c *authCache) set(keyHash string, user *models.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[keyHash]; ok {
		c.entries[keyHash] = &authCacheEntry{user: user, expiresAt: time.Now().Add(authCacheTTL)}
		c.touchLocked(keyHash)
		return
	}
	for len(c.order) >= authCacheMaxSize {
		c.removeLocked(c.order[0])
	}
	c.entries[keyHash] = &authCacheEntry{user: user, expiresAt: time.Now().Add(authCacheTTL)}
	c.order = append(c.order, keyHash)
}

func (c *authCache) touchLocked(keyHash string) {
	for i, k := range c.order {
		if k == keyHash {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.order = append(c.order, keyHash)
}

func (c *authCache) removeLocked(keyHash string) {
	delete(c.entries, keyHash)
	for i, k := range c.order {
		if k == keyHash {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}

// AuthMiddleware extracts the Bearer token from the Authorization header,
// resolves it to a User (with a short-lived in-process cache), and injects the
// user into the request context. Returns 401 for missing or invalid tokens.
func AuthMiddleware(us *store.UserStore) func(http.Handler) http.Handler {
	cache := newAuthCache()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if token == "" || token == auth {
				writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
				return
			}

			keyHash := store.HashAPIKey(token)
			if user, ok := cache.get(keyHash); ok {
				ctx := context.WithValue(r.Context(), ctxUser, user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			user, err := us.GetByAPIKey(r.Context(), token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			cache.set(keyHash, user)
			ctx := context.WithValue(r.Context(), ctxUser, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// currentUser retrieves the authenticated User from the request context.
// Returns nil only when called outside of AuthMiddleware (never in practice).
func currentUser(r *http.Request) *models.User {
	u, _ := r.Context().Value(ctxUser).(*models.User)
	return u
}
