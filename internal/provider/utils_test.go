package provider

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRateLimitError_Error(t *testing.T) {
	t.Run("WithRetryAfter", func(t *testing.T) {
		e := &RateLimitError{RetryAfter: 5 * time.Second}
		want := "rate limited, retry after 5s"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("WithoutRetryAfter", func(t *testing.T) {
		e := &RateLimitError{}
		want := "rate limited (429)"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("ZeroRetryAfter", func(t *testing.T) {
		e := &RateLimitError{RetryAfter: 0}
		want := "rate limited (429)"
		if got := e.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})
}

func TestIsRateLimitError(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		if IsRateLimitError(nil) {
			t.Error("IsRateLimitError(nil) should be false")
		}
	})

	t.Run("Direct", func(t *testing.T) {
		if !IsRateLimitError(&RateLimitError{}) {
			t.Error("IsRateLimitError(&RateLimitError{}) should be true")
		}
	})

	t.Run("WithRetryAfter", func(t *testing.T) {
		if !IsRateLimitError(&RateLimitError{RetryAfter: 30 * time.Second}) {
			t.Error("IsRateLimitError should detect RateLimitError with RetryAfter")
		}
	})

	t.Run("Wrapped", func(t *testing.T) {
		wrapped := fmt.Errorf("outer: %w", &RateLimitError{RetryAfter: 1 * time.Second})
		if !IsRateLimitError(wrapped) {
			t.Error("IsRateLimitError should detect wrapped RateLimitError")
		}
	})

	t.Run("OtherError", func(t *testing.T) {
		if IsRateLimitError(errors.New("some other error")) {
			t.Error("IsRateLimitError should be false for non-rate-limit errors")
		}
	})
}

func TestCleanScopes(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		if got := cleanScopes(nil); got != nil {
			t.Errorf("cleanScopes(nil) = %v, want nil", got)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		if got := cleanScopes([]string{}); got != nil {
			t.Errorf("cleanScopes([]) = %v, want nil", got)
		}
	})

	t.Run("DedupAndSort", func(t *testing.T) {
		got := cleanScopes([]string{"write", "read", "write", "follow"})
		want := []string{"follow", "read", "write"}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d; got %v", len(got), len(want), got)
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("got[%d] = %q, want %q", i, got[i], w)
			}
		}
	})

	t.Run("TrimSpaces", func(t *testing.T) {
		got := cleanScopes([]string{"  read  ", "write"})
		if len(got) != 2 || got[0] != "read" || got[1] != "write" {
			t.Errorf("cleanScopes with spaces: got %v", got)
		}
	})

	t.Run("RemoveBlanks", func(t *testing.T) {
		got := cleanScopes([]string{"", "  ", "read"})
		if len(got) != 1 || got[0] != "read" {
			t.Errorf("cleanScopes blanks: got %v, want [read]", got)
		}
	})

	t.Run("Single", func(t *testing.T) {
		got := cleanScopes([]string{"read"})
		if len(got) != 1 || got[0] != "read" {
			t.Errorf("cleanScopes single: got %v", got)
		}
	})
}
