package secretprovider

import (
	"context"
	"sync"
	"time"

	secrettypes "github.com/kimdre/doco-cd/internal/secretprovider/types"
)

// cacheEntry holds a cached secret value and its expiry time.
type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// CachingSecretProvider wraps a SecretProvider and caches resolved secret values
// for the configured TTL to avoid redundant upstream API calls.
type CachingSecretProvider struct {
	inner SecretProvider
	ttl   time.Duration
	mu    sync.RWMutex
	cache map[string]cacheEntry
}

// NewCachingSecretProvider wraps the given SecretProvider with a TTL-based
// in-memory cache. Each unique secret ID is cached individually; a cache miss
// or an expired entry triggers a fresh fetch from the upstream provider.
func NewCachingSecretProvider(inner SecretProvider, ttl time.Duration) *CachingSecretProvider {
	return &CachingSecretProvider{
		inner: inner,
		ttl:   ttl,
		cache: make(map[string]cacheEntry),
	}
}

// Name delegates to the wrapped provider.
func (c *CachingSecretProvider) Name() string {
	return c.inner.Name()
}

// Close delegates to the wrapped provider.
func (c *CachingSecretProvider) Close() {
	c.inner.Close()
}

// lookup returns the cached value for id if it exists and has not expired.
func (c *CachingSecretProvider) lookup(id string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}

	return entry.value, true
}

// store saves value for id with a fresh expiry timestamp.
func (c *CachingSecretProvider) store(id, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[id] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// GetSecret retrieves a single secret, returning the cached value if available.
// Errors from the upstream provider are never cached.
func (c *CachingSecretProvider) GetSecret(ctx context.Context, id string) (string, error) {
	if v, ok := c.lookup(id); ok {
		return v, nil
	}

	v, err := c.inner.GetSecret(ctx, id)
	if err != nil {
		return "", err
	}

	c.store(id, v)

	return v, nil
}

// GetSecrets retrieves multiple secrets, serving cached values where available
// and only fetching the remainder from the upstream provider in a single call.
func (c *CachingSecretProvider) GetSecrets(ctx context.Context, ids []string) (map[string]string, error) {
	result := make(map[string]string, len(ids))
	missing := make([]string, 0, len(ids))

	for _, id := range ids {
		if v, ok := c.lookup(id); ok {
			result[id] = v
		} else {
			missing = append(missing, id)
		}
	}

	if len(missing) == 0 {
		return result, nil
	}

	fetched, err := c.inner.GetSecrets(ctx, missing)
	if err != nil {
		return nil, err
	}

	for id, v := range fetched {
		c.store(id, v)

		result[id] = v
	}

	return result, nil
}

// ResolveSecretReferences resolves secret references using the cache for
// previously fetched values, only hitting the upstream for uncached IDs.
func (c *CachingSecretProvider) ResolveSecretReferences(ctx context.Context, secrets map[string]string) (secrettypes.ResolvedSecrets, error) {
	ids := make([]string, 0, len(secrets))
	for _, id := range secrets {
		ids = append(ids, id)
	}

	resolved, err := c.GetSecrets(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[string]string, len(secrets))
	for envVar, secretID := range secrets {
		if val, ok := resolved[secretID]; ok {
			out[envVar] = val
		} else {
			out[envVar] = ""
		}
	}

	return out, nil
}
