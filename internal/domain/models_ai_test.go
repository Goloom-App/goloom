package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTeamProfileValidate(t *testing.T) {
	t.Parallel()
	testTeamProfileValidate(t)
}

func TestAITeamProfileValidate(t *testing.T) {
	t.Parallel()
	testTeamProfileValidate(t)
}

func testTeamProfileValidate(t *testing.T) {

	tests := []struct {
		name    string
		profile TeamProfile
		wantErr bool
	}{
		{
			name: "valid profile",
			profile: TeamProfile{
				TeamID: "team-1",
				StyleMetadata: StyleMetadata{
					Tonality:          "friendly",
					FormattingRules:   []string{"short paragraphs"},
					BannedWords:       []string{"spam"},
					MaxHashtags:       3,
					PreferredLanguage: "en",
				},
			},
			wantErr: false,
		},
		{
			name:    "empty team id",
			profile: TeamProfile{StyleMetadata: StyleMetadata{Tonality: "friendly"}},
			wantErr: true,
		},
		{
			name:    "nil style metadata",
			profile: TeamProfile{TeamID: "team-1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.profile.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAIModelSerialization(t *testing.T) {
	t.Parallel()

	profile := TeamProfile{
		ID:     "profile-1",
		TeamID: "team-1",
		StyleMetadata: StyleMetadata{
			Tonality:          "friendly",
			FormattingRules:   []string{"use bullets"},
			BannedWords:       []string{"spam"},
			MaxHashtags:       5,
			PreferredLanguage: "de",
		},
		AutoPublishEnabled: true,
		CreatedAt:          time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 5, 29, 11, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"id", "team_id", "style_metadata", "auto_publish_enabled", "created_at", "updated_at"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing json key %q in %s", key, string(data))
		}
	}
}

func TestAIJobTypeConstants(t *testing.T) {
	t.Parallel()

	tests := map[AIJobType]string{
		AIJobTypeVoiceEngine:       "voice_engine",
		AIJobTypeCampaignAutopilot: "campaign_autopilot",
		AIJobTypeProactiveTrigger:  "proactive_trigger",
		AIJobTypeVibePreview:       "vibe_preview",
		AIJobTypeProfileAssistant:  "profile_assistant",
	}

	for got, want := range tests {
		if string(got) != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
}
