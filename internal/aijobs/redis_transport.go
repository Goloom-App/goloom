package aijobs

import (
	"context"
	"fmt"

	"git.f4mily.net/goloom/internal/domain"
)

// Activate via config: set TRANSPORT=redis
type RedisTransport struct{}

func (t *RedisTransport) Dispatch(ctx context.Context, job domain.AIJob, serviceURL string) error {
	_ = ctx
	_ = job
	_ = serviceURL
	return fmt.Errorf("redis transport: not implemented")
}
