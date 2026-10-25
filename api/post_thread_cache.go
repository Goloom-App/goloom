package api

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"git.f4mily.net/goloom/internal/provider"
)

const (
	threadCacheTTL     = 30 * time.Second
	threadCacheMaxKeys = 256
)

type threadCacheEntry struct {
	result    provider.ThreadResult
	fetchedAt time.Time
}

// threadCache throttles upstream thread fetches: successes are cached for threadCacheTTL and
// concurrent misses for the same key share one upstream call. Errors are never cached.
// Callers must authorize access before calling get; the key only scopes data per team/post/account.
type threadCache struct {
	mu      sync.Mutex
	entries map[string]threadCacheEntry
	group   singleflight.Group
	now     func() time.Time
}

func newThreadCache() *threadCache {
	return &threadCache{entries: map[string]threadCacheEntry{}, now: time.Now}
}

func (c *threadCache) get(ctx context.Context, key string, fetch func(context.Context) (provider.ThreadResult, error)) (provider.ThreadResult, time.Time, error) {
	if e, ok := c.lookup(key); ok {
		return e.result, e.fetchedAt, nil
	}
	v, err, _ := c.group.Do(key, func() (any, error) {
		if e, ok := c.lookup(key); ok {
			return e, nil
		}
		// The shared call must survive cancellation of the request that happened to start it.
		res, err := fetch(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		e := threadCacheEntry{result: res, fetchedAt: c.now().UTC()}
		c.store(key, e)
		return e, nil
	})
	if err != nil {
		return provider.ThreadResult{}, time.Time{}, err
	}
	e := v.(threadCacheEntry)
	return e.result, e.fetchedAt, nil
}

func (c *threadCache) lookup(key string) (threadCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().Sub(e.fetchedAt) >= threadCacheTTL {
		delete(c.entries, key)
		return threadCacheEntry{}, false
	}
	return e, true
}

func (c *threadCache) store(key string, e threadCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= threadCacheMaxKeys {
		for k, v := range c.entries {
			if c.now().Sub(v.fetchedAt) >= threadCacheTTL {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= threadCacheMaxKeys {
			clear(c.entries)
		}
	}
	c.entries[key] = e
}
