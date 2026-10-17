package store

import (
	"context"
	"git.f4mily.net/goloom/internal/store/txcontext"
)

// AfterAuditCommit defers side effects until successful outer commit. The supplied
// context is safe for background work: it contains no committed transaction.
func AfterAuditCommit(ctx context.Context, fn func(context.Context)) { txcontext.AfterCommit(ctx, fn) }
