package secretprovider

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	secrettypes "github.com/kimdre/doco-cd/internal/secretprovider/types"
)

// mockProvider is a test double that counts upstream calls and returns preset values.
type mockProvider struct {
	calls  atomic.Int64
	values map[string]string
	err    error
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Close() {}

func (m *mockProvider) GetSecret(_ context.Context, id string) (string, error) {
	m.calls.Add(1)

	if m.err != nil {
		return "", m.err
	}

	return m.values[id], nil
}

func (m *mockProvider) GetSecrets(_ context.Context, ids []string) (map[string]string, error) {
	m.calls.Add(1)

	if m.err != nil {
		return nil, m.err
	}

	result := make(map[string]string, len(ids))
	for _, id := range ids {
		result[id] = m.values[id]
	}

	return result, nil
}

func (m *mockProvider) ResolveSecretReferences(ctx context.Context, secrets map[string]string) (secrettypes.ResolvedSecrets, error) {
	ids := make([]string, 0, len(secrets))
	for _, id := range secrets {
		ids = append(ids, id)
	}

	resolved, err := m.GetSecrets(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[string]string, len(secrets))
	for envVar, id := range secrets {
		out[envVar] = resolved[id]
	}

	return out, nil
}

func TestCachingSecretProvider_HitAfterFirstFetch(t *testing.T) {
	t.Parallel()

	mock := &mockProvider{
		values: map[string]string{"op://vault/item/field": "hunter2"},
	}
	c := NewCachingSecretProvider(mock, time.Minute)

	// First call: cache miss — upstream must be called.
	v, err := c.GetSecret(t.Context(), "op://vault/item/field")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v != "hunter2" {
		t.Fatalf("expected 'hunter2', got %q", v)
	}

	if mock.calls.Load() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", mock.calls.Load())
	}

	// Second call: cache hit — upstream must NOT be called again.
	v, err = c.GetSecret(t.Context(), "op://vault/item/field")
	if err != nil {
		t.Fatalf("unexpected error on cached call: %v", err)
	}

	if v != "hunter2" {
		t.Fatalf("expected 'hunter2' from cache, got %q", v)
	}

	if mock.calls.Load() != 1 {
		t.Fatalf("expected still 1 upstream call after cache hit, got %d", mock.calls.Load())
	}
}

func TestCachingSecretProvider_ExpiredEntryRefetched(t *testing.T) {
	t.Parallel()

	mock := &mockProvider{
		values: map[string]string{"op://vault/item/field": "hunter2"},
	}
	c := NewCachingSecretProvider(mock, time.Millisecond)

	_, _ = c.GetSecret(t.Context(), "op://vault/item/field")

	// Wait for the TTL to expire.
	time.Sleep(10 * time.Millisecond)

	_, err := c.GetSecret(t.Context(), "op://vault/item/field")
	if err != nil {
		t.Fatalf("unexpected error after TTL expiry: %v", err)
	}

	if mock.calls.Load() != 2 {
		t.Fatalf("expected 2 upstream calls after TTL expiry, got %d", mock.calls.Load())
	}
}

func TestCachingSecretProvider_GetSecretsPartialCache(t *testing.T) {
	t.Parallel()

	mock := &mockProvider{
		values: map[string]string{
			"op://vault/item/a": "secret-a",
			"op://vault/item/b": "secret-b",
		},
	}
	c := NewCachingSecretProvider(mock, time.Minute)

	// Pre-warm the cache for "a" only.
	_, _ = c.GetSecret(t.Context(), "op://vault/item/a")
	callsBefore := mock.calls.Load()

	// GetSecrets for both "a" (cached) and "b" (not cached).
	result, err := c.GetSecrets(t.Context(), []string{"op://vault/item/a", "op://vault/item/b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["op://vault/item/a"] != "secret-a" {
		t.Fatalf("unexpected value for a: %q", result["op://vault/item/a"])
	}

	if result["op://vault/item/b"] != "secret-b" {
		t.Fatalf("unexpected value for b: %q", result["op://vault/item/b"])
	}

	// Exactly one additional upstream call — only for "b".
	if mock.calls.Load() != callsBefore+1 {
		t.Fatalf("expected exactly one extra upstream call for the missing key, got %d total", mock.calls.Load())
	}
}

func TestCachingSecretProvider_AllCached_NoUpstreamCall(t *testing.T) {
	t.Parallel()

	mock := &mockProvider{
		values: map[string]string{
			"op://vault/item/a": "secret-a",
			"op://vault/item/b": "secret-b",
		},
	}
	c := NewCachingSecretProvider(mock, time.Minute)

	// Warm the cache for both.
	_, _ = c.GetSecrets(t.Context(), []string{"op://vault/item/a", "op://vault/item/b"})
	callsBefore := mock.calls.Load()

	// All entries are cached — upstream must not be called.
	_, err := c.GetSecrets(t.Context(), []string{"op://vault/item/a", "op://vault/item/b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.calls.Load() != callsBefore {
		t.Fatalf("expected no extra upstream calls when all secrets are cached, got %d extra", mock.calls.Load()-callsBefore)
	}
}

func TestCachingSecretProvider_UpstreamError_NotCached(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("upstream failure")
	mock := &mockProvider{err: wantErr}
	c := NewCachingSecretProvider(mock, time.Minute)

	_, err := c.GetSecret(t.Context(), "op://vault/item/field")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected upstream error, got %v", err)
	}

	// A subsequent call must still hit upstream — errors must not be cached.
	_, _ = c.GetSecret(t.Context(), "op://vault/item/field")

	if mock.calls.Load() != 2 {
		t.Fatalf("expected 2 upstream calls when errors are not cached, got %d", mock.calls.Load())
	}
}

func TestCachingSecretProvider_ResolveSecretReferences(t *testing.T) {
	t.Parallel()

	mock := &mockProvider{
		values: map[string]string{
			"op://vault/item/db_pass": "s3cr3t",
		},
	}
	c := NewCachingSecretProvider(mock, time.Minute)

	secrets := map[string]string{
		"DB_PASSWORD": "op://vault/item/db_pass",
	}

	resolved, err := c.ResolveSecretReferences(t.Context(), secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved["DB_PASSWORD"] != "s3cr3t" {
		t.Fatalf("expected 's3cr3t', got %q", resolved["DB_PASSWORD"])
	}

	// Second resolve — must come from cache.
	_, _ = c.ResolveSecretReferences(t.Context(), secrets)

	if mock.calls.Load() != 1 {
		t.Fatalf("expected 1 upstream call total after two resolves, got %d", mock.calls.Load())
	}
}
