// package cache provides constructs to enable caching of commonly required resources from the 3scale 'system' APIs
package cache

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/3scale/3scale-porta-go-client/client"
	"github.com/orcaman/concurrent-map"
)

const (
	// DefaultCacheTTL - Default time to wait before marking cached values expired
	DefaultCacheTTL = time.Duration(time.Minute * 5)

	// DefaultCacheRefreshInterval - Default interval at which background process should refresh items
	DefaultCacheRefreshInterval = time.Duration(time.Minute * 3)

	// DefaultCacheLimit - Default max number of items that can be stored in the cache at any time
	// A negative value implies that there is no limit on the number of cached items
	DefaultCacheLimit = -1
)

var now = time.Now

// ConfigurationCache is the interface for managing a cache of `Proxy Config` resource(s)
type ConfigurationCache interface {
	// Get retrieves an element from the cache (if present) and returns a result, as well as a boolean value
	// which identifies if the element was present or not
	Get(key string) (Value, bool)
	Set(key string, value Value) error
	Delete(key string)
	FlushExpired()
	Refresh()
}

// Value defines the value that must be stored in the cache
type Value struct {
	Item        client.ProxyConfig
	expires     time.Time
	refreshWith RefreshCb
}

// ConfigCache provides an in-memory solution which implements 'ConfigurationCache'
type ConfigCache struct {
	cache                cmap.ConcurrentMap
	limit                int
	refreshWorkerRunning int32
	stopRefreshWorker    chan struct{}
	ttl                  time.Duration
}

// RefreshCb defines a callback which can be used to refresh elements in the cache as required
type RefreshCb func() (client.ProxyConfig, error)

// NewConfigCache returns a ConfigCache configured with the provided inputs
// It accepts a 'time to live' which will be the default value used to mark cached items as expired
// Max entries limits the number of objects that can exist in the cache at a given time
func NewConfigCache(ttl time.Duration, maxEntries int) *ConfigCache {
	return &ConfigCache{
		limit: maxEntries,
		ttl:   ttl,
		cache: cmap.New(),
	}
}

// NewDefaultConfigCache returns a ConfigCache configured with the default values
func NewDefaultConfigCache() *ConfigCache {
	return &ConfigCache{
		limit: DefaultCacheLimit,
		ttl:   DefaultCacheTTL,
		cache: cmap.New(),
	}
}

// Get an element from the cache if it exists
// The returned bool identifies if the element was present or not
func (scp *ConfigCache) Get(key string) (Value, bool) {
	value, ok := scp.cache.Get(key)
	if !ok {
		return Value{}, ok
	}
	return value.(Value), ok
}

// Set an item in the cache under the provided key
// Returns an error if the max number of entries in the cache has been reached
func (scp *ConfigCache) Set(key string, v Value) error {
	if scp.limit < 0 || scp.cache.Count() < scp.limit {
		if v.expires.IsZero() {
			v.expires = scp.getExpiryTime()
		}
		scp.cache.Set(key, v)
		return nil
	}

	return errors.New("error - cache is full, cannot add more elements")
}

// Delete an element from the cache
func (scp *ConfigCache) Delete(key string) {
	scp.cache.Remove(key)
}

// FlushExpired elements from the cache
// Any element whose expiration date is passed the current time will be removed immediately
func (scp *ConfigCache) FlushExpired() {
	var forDeletion []string

	scp.cache.IterCb(func(key string, v interface{}) {
		item := v.(Value)
		if item.isExpired() {
			forDeletion = append(forDeletion, key)
		}
	})
	for _, key := range forDeletion {
		scp.Delete(key)
	}
}

// Refresh elements in the cache using the provided callback
// Elements whose callback returns an error will not be refreshed but wil be left in the cache to expire
func (scp *ConfigCache) Refresh() {
	refreshItems := make(map[string]Value)

	scp.cache.IterCb(func(key string, v interface{}) {
		item := v.(Value)
		if item.refreshWith != nil {
			resp, err := item.refreshWith()
			if err != nil {
				return
			}

			value := Value{
				Item:        resp,
				expires:     scp.getExpiryTime(),
				refreshWith: item.refreshWith,
			}
			refreshItems[key] = value
		}
	})
	for k, v := range refreshItems {
		scp.Set(k, v)
	}
}

// RunRefreshWorker at increments provided by the interval
// At each interval, elements will be refreshed. See 'Refresh()'
func (scp *ConfigCache) RunRefreshWorker(interval time.Duration, stop chan struct{}) error {
	if !atomic.CompareAndSwapInt32(&scp.refreshWorkerRunning, 0, 1) {
		return errors.New("worker has already been started")
	}

	scp.stopRefreshWorker = stop
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				scp.Refresh()
			case <-stop:
				ticker.Stop()
				return
			}
		}
	}()
	return nil
}

func (scp *ConfigCache) getExpiryTime() time.Time {
	return now().Add(scp.ttl)
}

// SetExpiry time on a value to override the default expiry time set by the caching implementation
func (v *Value) SetExpiry(t time.Time) *Value {
	v.expires = t
	return v
}

// SetRefreshCallback, the callback that will be used to attempt to refresh an element when requested
// Retry and backoff logic should be implemented in the callback as required.
func (v *Value) SetRefreshCallback(fn RefreshCb) *Value {
	v.refreshWith = fn
	return v
}

func (v Value) isExpired() bool {
	return now().After(v.expires)
}
