// Package cache is a small in-memory TTL cache for HUD segment values. Unlike
// the v2.1 file cache, v3.1 is a long-lived process, so values live in memory
// guarded by a mutex; no files or file locks are needed.
package cache

import (
	"sync"
	"time"
)

type item struct {
	val     string
	expires time.Time
	ok      bool
}

// Cache is a concurrency-safe key->string store with per-call TTL.
type Cache struct {
	mu sync.Mutex
	m  map[string]item
}

// New returns an empty Cache.
func New() *Cache { return &Cache{m: make(map[string]item)} }

// GetOrRefresh returns the cached value for key if it is still within ttl.
// Otherwise it calls refresh, stores the result, and returns it. On a refresh
// error it returns the previous (stale) value if one exists, else "". Holds the
// lock for the whole call (single-flight); refresh implementations MUST bound
// their own time (HTTP/exec timeouts).
func (c *Cache) GetOrRefresh(key string, ttl time.Duration, refresh func() (string, error)) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if it, found := c.m[key]; found && it.ok && now.Before(it.expires) {
		return it.val
	}

	val, err := refresh()
	if err != nil {
		if it, found := c.m[key]; found && it.ok {
			return it.val
		}
		return ""
	}
	c.m[key] = item{val: val, expires: now.Add(ttl), ok: true}
	return val
}
