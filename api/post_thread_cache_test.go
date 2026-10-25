package api

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/provider"
)

func TestThreadCache_SingleflightTTLAndErrors(t *testing.T) {
	c := newThreadCache()
	now := time.Now()
	c.now = func() time.Time { return now }
	var calls int32
	release := make(chan struct{})
	fetch := func(context.Context) (provider.ThreadResult, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return provider.ThreadResult{Truncated: true}, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := c.get(context.Background(), "k", fetch); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("concurrent calls = %d, want 1", got)
	}
	if _, _, err := c.get(context.Background(), "k", fetch); err != nil || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("cached get refetched (calls=%d err=%v)", calls, err)
	}
	now = now.Add(threadCacheTTL + time.Second)
	if _, _, err := c.get(context.Background(), "k", fetch); err != nil || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expired entry not refetched (calls=%d err=%v)", calls, err)
	}

	boom := errors.New("boom")
	failing := func(context.Context) (provider.ThreadResult, error) {
		atomic.AddInt32(&calls, 1)
		return provider.ThreadResult{}, boom
	}
	for i := 0; i < 2; i++ {
		if _, _, err := c.get(context.Background(), "e", failing); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Fatalf("errors must not be cached, calls = %d", got)
	}
}
