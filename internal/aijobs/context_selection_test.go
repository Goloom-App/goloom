package aijobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
)

type contextRunner struct{ got domain.AIContext }

func (r *contextRunner) RunJob(_ context.Context, _ domain.AIJob, _ domain.AIServiceConfig, c domain.AIContext) (json.RawMessage, error) {
	r.got = c
	return json.RawMessage(`{}`), nil
}

func TestJobsApplyKnowledgeBeforeDispatch(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	selected, err := s.CreateKnowledgeSource(ctx, team.ID, domain.KnowledgeSource{Name: "Selected", Type: "text", Content: "Relevant"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateKnowledgeSource(ctx, team.ID, domain.KnowledgeSource{Name: "Other", Type: "text", Content: "Unrelated"})
	if err != nil {
		t.Fatal(err)
	}
	format, err := s.CreateCampaignFormat(ctx, team.ID, domain.CampaignFormat{Name: "Selection", KnowledgeSourceIDs: []string{selected.ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{domain.AIJobOriginUser, domain.AIJobOriginAutomation, domain.AIJobOriginPreview} {
		for _, p := range []map[string]any{{"knowledge_source_ids": []string{selected.ID}}, {"campaign_format_id": format.ID}, {"knowledge_source_ids": []string{}}} {
			runner := &contextRunner{}
			manager := aijobs.NewManager(s, runner)
			payload, _ := json.Marshal(map[string]any{"params": p})
			_, err := manager.SubmitJob(ctx, domain.AIJob{Origin: origin, TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			manager.Wait()
			want := 1
			if ids, ok := p["knowledge_source_ids"].([]string); ok && len(ids) == 0 {
				want = 0
			}
			if len(runner.got.KnowledgeSources) != want || (want > 0 && runner.got.KnowledgeSources[0].ID != selected.ID) {
				t.Fatalf("%s: %+v", origin, runner.got.KnowledgeSources)
			}
		}
	}
	if err := s.DeleteKnowledgeSource(ctx, team.ID, selected.ID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"campaign_format_id": format.ID}})
	_, err = aijobs.NewManager(s, &contextRunner{}).SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
	if !errors.Is(err, aicontext.ErrKnowledgeSourceUnavailable) {
		t.Fatalf("deleted selection dispatched: %v", err)
	}
}
