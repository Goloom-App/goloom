package aijobs

import (
	"context"

	"git.f4mily.net/goloom/internal/domain"
)

type Transport interface {
	Dispatch(ctx context.Context, job domain.AIJob, serviceURL string) error
}
