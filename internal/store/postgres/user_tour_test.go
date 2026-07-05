package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Mirror of the sqlite suite: the tour flag is per user, persists across
// re-login upserts, and is readable on every user read path.
func TestPostgres_SetUserTourDone(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	subject := "tour-" + uuid.NewString()
	u, err := s.UpsertOIDCUser(ctx, subject, "tour@pg.test", "Tour")
	if err != nil {
		t.Fatal(err)
	}
	if u.TourDone {
		t.Fatalf("new user must start with tour_done = false: %+v", u)
	}

	updated, err := s.SetUserTourDone(ctx, u.ID, true)
	if err != nil || !updated.TourDone {
		t.Fatalf("SetUserTourDone(true): %+v %v", updated, err)
	}

	again, err := s.UpsertOIDCUser(ctx, subject, "tour@pg.test", "Tour")
	if err != nil || !again.TourDone {
		t.Fatalf("tour_done lost on upsert: %+v %v", again, err)
	}

	reset, err := s.SetUserTourDone(ctx, u.ID, false)
	if err != nil || reset.TourDone {
		t.Fatalf("SetUserTourDone(false): %+v %v", reset, err)
	}
}
