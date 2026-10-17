// Package txcontext coordinates side effects after the outer transaction commits.
package txcontext

import (
	"context"
	"sync"
)

type key struct{}
type hooks struct {
	mu        sync.Mutex
	callbacks []func(context.Context)
}

// AfterCommit invokes fn immediately outside a transaction. Inside, fn receives
// the original context without transaction values after the outer commit succeeds.
func AfterCommit(ctx context.Context, fn func(context.Context)) {
	if h, ok := ctx.Value(key{}).(*hooks); ok {
		h.mu.Lock()
		h.callbacks = append(h.callbacks, fn)
		h.mu.Unlock()
		return
	}
	fn(ctx)
}

// Begin gives each savepoint its own hooks, so rolling one back discards only its
// side effects. Successful nested hooks transfer to the parent transaction.
func Begin(ctx context.Context) (context.Context, func(bool)) {
	h := &hooks{}
	return context.WithValue(ctx, key{}, h), func(success bool) {
		if !success {
			return
		}
		h.mu.Lock()
		callbacks := h.callbacks
		h.callbacks = nil
		h.mu.Unlock()
		for _, fn := range callbacks {
			AfterCommit(ctx, fn)
		}
	}
}
