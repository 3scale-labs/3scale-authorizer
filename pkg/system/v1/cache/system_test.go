package cache

import (
	"net/http"
	"testing"
	"time"

	"github.com/3scale/3scale-porta-go-client/client"
)

func TestConfigCache_Get(t *testing.T) {
	cc := NewDefaultConfigCache()
	if _, ok := cc.Get("non-existent"); ok {
		t.Error("expected function to return false when item not present")
	}

	cc.Set("test", Value{Item: client.ProxyConfig{ID: 5}})
	v, ok := cc.Get("test")
	if !ok {
		t.Error("expected element to be present")
	}

	if v.Item.ID != 5 {
		t.Error("unexpected value for ID of cached item")
	}
}

func TestConfigCache_Set(t *testing.T) {
	cc := NewDefaultConfigCache()
	if cc.cache.Count() != 0 {
		t.Error("expected new cache to be empty")
	}

	cc.Set("test", Value{Item: client.ProxyConfig{ID: 5}})
	if cc.cache.Count() != 1 {
		t.Error("expected cache to have only one element")
	}

	cc = NewConfigCache(time.Hour, -1)
	err := cc.Set("any", Value{})
	if err != nil {
		t.Error("expected negative cache limit to be limitless")
	}

	cc = NewConfigCache(time.Hour, 0)
	if err = cc.Set("any", Value{}); err == nil {
		t.Error("expected zero cache limit to be restricted")
	}

	cc = NewConfigCache(time.Hour, 1)
	if err = cc.Set("any", Value{}); err != nil {
		t.Error("expected first insert to be a success")
	}
	if err = cc.Set("second", Value{}); err == nil {
		t.Error("expected second insert to fail")
	}
}

func TestConfigCache_Delete(t *testing.T) {
	cc := NewDefaultConfigCache()
	cc.Set("test", Value{Item: client.ProxyConfig{ID: 5}})
	if cc.cache.Count() != 1 {
		t.Error("expected cache to have only one element")
	}

	cc.Delete("test")
	if cc.cache.Count() != 0 {
		t.Error("expected cache to have no elements post deletion")
	}
}

func TestConfigCache_FlushExpired(t *testing.T) {
	cc := NewDefaultConfigCache()

	yesterday := time.Hour * -24
	v := Value{Item: client.ProxyConfig{ID: 5}}
	v.SetExpiry(time.Now().Add(yesterday))

	cc.Set("test", v)
	if cc.cache.Count() != 1 {
		t.Error("expected cache to have only one element")
	}

	cc.FlushExpired()
	if cc.cache.Count() != 0 {
		t.Error("expected cache to be empty after flushing expired items")
	}
}

func TestConfigCache_Refresh(t *testing.T) {
	cc := NewDefaultConfigCache()

	// test value unmodified during error
	refreshCb := func() (client.ProxyConfig, error) {
		return client.ProxyConfig{}, http.ErrHandlerTimeout
	}
	v := Value{Item: client.ProxyConfig{ID: 5}}
	v.SetRefreshCallback(refreshCb)

	cc.Set("test", v)
	cc.Refresh()
	updatedV, ok := cc.Get("test")
	if !ok {
		t.Error("expected element to be present")
	}
	if updatedV.Item.ID != 5 {
		t.Error("unexpected result. expected callback to have modified ID when refreshing")
	}

	// test refresh success
	refreshCb = func() (client.ProxyConfig, error) {
		return client.ProxyConfig{ID: 6}, nil
	}
	v = Value{Item: client.ProxyConfig{ID: 5}}
	v.SetRefreshCallback(refreshCb)

	cc.Set("test", v)
	cc.Refresh()
	updatedV, ok = cc.Get("test")
	if !ok {
		t.Error("expected element to be present")
	}
	if updatedV.Item.ID != 6 {
		t.Error("unexpected result. expected callback to have modified ID when refreshing")
	}
}

func TestConfigCache_RunRefreshWorker(t *testing.T) {
	// test error on startup
	cc := NewDefaultConfigCache()
	cc.refreshWorkerRunning = 1
	if err := cc.RunRefreshWorker(time.Hour, nil); err == nil {
		t.Error("expected error as worker had been marked as started")
	}

	cc = NewDefaultConfigCache()
	done := make(chan bool)
	refreshCb := func() (client.ProxyConfig, error) {
		done <- true
		return client.ProxyConfig{ID: 0}, nil
	}

	v := Value{}
	v.SetRefreshCallback(refreshCb)
	cc.Set("test", v)
	stop := make(chan struct{})
	if err := cc.RunRefreshWorker(time.Millisecond, stop); err != nil {
		t.Errorf("unexpected error when running refresh worker")
	}

	<-time.After(time.Second)
	success := <-done
	if !success {
		t.Error("expected refresh worker to have been called an callback to be executed")
	}

	close(stop)

}
