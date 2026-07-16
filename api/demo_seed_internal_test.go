package api

import (
	"context"
	"errors"
	"testing"
)

func TestRetryDemoSeedWriteRetriesSQLiteBusy(t *testing.T) {
	attempts := 0
	err := retryDemoSeedWrite(context.Background(), func() error {
		attempts++
		if attempts == 1 {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryDemoSeedWrite returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRetryDemoSeedWriteDoesNotRetryPermanentError(t *testing.T) {
	attempts := 0
	want := errors.New("validation failed")
	err := retryDemoSeedWrite(context.Background(), func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
