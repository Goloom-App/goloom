package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func TestAIJobOrigin(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := newAITestTeam(t, s)
	for _, origin := range []string{"", domain.AIJobOriginAutomation, domain.AIJobOriginPreview} {
		input := domain.AIJob{TeamID: team.ID, AuthorUserID: u.ID, Type: domain.AIJobTypeVoiceEngine, Origin: origin}
		job, err := s.CreateAIJob(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		want := origin
		if want == "" {
			want = "user"
		}
		if job.Origin != want {
			t.Fatalf("origin=%q, want %q", job.Origin, want)
		}
		raw, _ := json.Marshal(job)
		var decoded map[string]any
		json.Unmarshal(raw, &decoded)
		if _, ok := decoded["origin"]; ok {
			t.Fatal("origin exposed in JSON")
		}
		byID, err := s.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil || byID.Origin != want {
			t.Fatalf("global origin: %+v %v", byID, err)
		}
		for name, list := range map[string]func() ([]domain.AIJob, error){"team": func() ([]domain.AIJob, error) { return s.ListAIJobs(ctx, team.ID, 100) }, "pending": func() ([]domain.AIJob, error) { return s.ListPendingAIJobs(ctx, 10000) }, "unfinished": func() ([]domain.AIJob, error) { return s.ListUnfinishedAIJobs(ctx, time.Now().Add(time.Hour), 10000) }} {
			jobs, err := list()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, got := range jobs {
				if got.ID == job.ID {
					found = true
					if got.Origin != want {
						t.Fatalf("%s origin=%q", name, got.Origin)
					}
				}
			}
			if !found {
				t.Fatalf("%s missing job", name)
			}
		}
		if err := s.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusCompleted, nil, ""); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetAIJobByID(ctx, team.ID, job.ID)
		if err != nil || got.Origin != want {
			t.Fatalf("completion changed origin: %+v %v", got, err)
		}
	}
}

func TestAIJobOriginLegacyMigration(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := newAITestTeam(t, s)
	job, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: u.ID, Type: domain.AIJobTypeVoiceEngine, Payload: json.RawMessage(`{"rss_automation":{}}`)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err = conn.Exec(ctx, `alter table ai_jobs drop column origin`); err != nil {
		t.Fatal(err)
	}
	reopened := newTestStore(t)
	got, err := reopened.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Origin != "user" {
		t.Fatalf("legacy origin=%q, want user", got.Origin)
	}
}
