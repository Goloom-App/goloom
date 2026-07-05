package sqlite_test

import (
	"context"
	"testing"
)

// The guided tour's "done" flag must live on the user, not in the browser:
// completing it on one device has to stick on every other device, and a new
// user on a shared browser must still get the tour.
func TestSQLite_SetUserTourDone(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	u, err := s.UpsertOIDCUser(ctx, "tour-user", "tour@x", "Tour")
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

	// The flag must survive a re-login (upsert) and show up on every read path.
	again, err := s.UpsertOIDCUser(ctx, "tour-user", "tour@x", "Tour")
	if err != nil || !again.TourDone {
		t.Fatalf("tour_done lost on upsert: %+v %v", again, err)
	}
	list, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range list {
		if item.ID == u.ID {
			found = true
			if !item.TourDone {
				t.Fatalf("ListUsers lost tour_done: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("user missing from ListUsers")
	}

	reset, err := s.SetUserTourDone(ctx, u.ID, false)
	if err != nil || reset.TourDone {
		t.Fatalf("SetUserTourDone(false): %+v %v", reset, err)
	}
}
