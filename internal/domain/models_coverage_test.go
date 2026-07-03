package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// --- FilterMediaIDsForAccount ---

func TestFilterMediaIDsForAccount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		all       []string
		exclude   map[string][]string
		accountID string
		want      []string
	}{
		{
			name:      "no exclusions",
			all:       []string{"m1", "m2"},
			exclude:   nil,
			accountID: "acc1",
			want:      []string{"m1", "m2"},
		},
		{
			name:      "empty media list",
			all:       []string{},
			exclude:   map[string][]string{"acc1": {"m1"}},
			accountID: "acc1",
			want:      []string{},
		},
		{
			name:      "exclude one for account",
			all:       []string{"m1", "m2", "m3"},
			exclude:   map[string][]string{"acc1": {"m2"}},
			accountID: "acc1",
			want:      []string{"m1", "m3"},
		},
		{
			name:      "exclusion for different account does not apply",
			all:       []string{"m1", "m2"},
			exclude:   map[string][]string{"other": {"m1"}},
			accountID: "acc1",
			want:      []string{"m1", "m2"},
		},
		{
			name:      "all media excluded",
			all:       []string{"m1", "m2"},
			exclude:   map[string][]string{"acc1": {"m1", "m2"}},
			accountID: "acc1",
			want:      []string{},
		},
		{
			name:      "account id is trimmed",
			all:       []string{"m1"},
			exclude:   map[string][]string{"acc1": {"m1"}},
			accountID: "  acc1  ",
			want:      []string{},
		},
		{
			name:      "whitespace-only media IDs are dropped",
			all:       []string{" ", "m1"},
			exclude:   nil,
			accountID: "acc1",
			want:      []string{"m1"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FilterMediaIDsForAccount(tc.all, tc.exclude, tc.accountID)
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d want %d (got=%v want=%v)", len(got), len(tc.want), got, tc.want)
			}
			for i, v := range got {
				if v != tc.want[i] {
					t.Errorf("index %d: got %q want %q", i, v, tc.want[i])
				}
			}
		})
	}
}

// --- NormalizeRSSInitialSyncMode ---

func TestNormalizeRSSInitialSyncMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want RSSInitialSyncMode
	}{
		{"baseline", RSSInitialSyncBaseline},
		{"BASELINE", RSSInitialSyncBaseline},
		{"publish_latest", RSSInitialSyncPublishLatest},
		{"PUBLISH_LATEST", RSSInitialSyncPublishLatest},
		{"  publish_latest  ", RSSInitialSyncPublishLatest},
		{"unknown", RSSInitialSyncBaseline},
		{"", RSSInitialSyncBaseline},
	}
	for _, tc := range cases {
		if got := NormalizeRSSInitialSyncMode(tc.raw); got != tc.want {
			t.Errorf("NormalizeRSSInitialSyncMode(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// --- NormalizeAutomationOutputMode ---

func TestNormalizeAutomationOutputMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want AutomationOutputMode
	}{
		{"draft", AutomationOutputDraft},
		{"DRAFT", AutomationOutputDraft},
		{"scheduled", AutomationOutputScheduled},
		{"SCHEDULED", AutomationOutputScheduled},
		{"  scheduled  ", AutomationOutputScheduled},
		{"publish_now", AutomationOutputPublishNow},
		{"PUBLISH_NOW", AutomationOutputPublishNow},
		{"unknown", AutomationOutputDraft},
		{"", AutomationOutputDraft},
	}
	for _, tc := range cases {
		if got := NormalizeAutomationOutputMode(tc.raw); got != tc.want {
			t.Errorf("NormalizeAutomationOutputMode(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// --- RSSFeedConfig.NormalizedContentTemplate / NormalizedTitleTemplate / NormalizedMaxPostsPerDay ---

func TestRSSFeedConfig_NormalizedTemplates(t *testing.T) {
	t.Parallel()

	t.Run("ContentTemplate defaults when blank", func(t *testing.T) {
		f := RSSFeedConfig{ContentTemplate: "   "}
		if got := f.NormalizedContentTemplate(); got != DefaultRSSContentTemplate {
			t.Errorf("got %q, want default %q", got, DefaultRSSContentTemplate)
		}
	})
	t.Run("ContentTemplate preserved when set", func(t *testing.T) {
		f := RSSFeedConfig{ContentTemplate: "custom {title}"}
		if got := f.NormalizedContentTemplate(); got != "custom {title}" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("TitleTemplate defaults when empty", func(t *testing.T) {
		f := RSSFeedConfig{}
		if got := f.NormalizedTitleTemplate(); got != DefaultRSSTitleTemplate {
			t.Errorf("got %q, want default %q", got, DefaultRSSTitleTemplate)
		}
	})
	t.Run("TitleTemplate preserved when set", func(t *testing.T) {
		f := RSSFeedConfig{TitleTemplate: "EP {counter}"}
		if got := f.NormalizedTitleTemplate(); got != "EP {counter}" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("MaxPostsPerDay defaults to 10 for zero", func(t *testing.T) {
		f := RSSFeedConfig{MaxPostsPerDay: 0}
		if got := f.NormalizedMaxPostsPerDay(); got != 10 {
			t.Errorf("got %d, want 10", got)
		}
	})
	t.Run("MaxPostsPerDay defaults to 10 for negative", func(t *testing.T) {
		f := RSSFeedConfig{MaxPostsPerDay: -5}
		if got := f.NormalizedMaxPostsPerDay(); got != 10 {
			t.Errorf("got %d, want 10", got)
		}
	})
	t.Run("MaxPostsPerDay preserved when positive", func(t *testing.T) {
		f := RSSFeedConfig{MaxPostsPerDay: 3}
		if got := f.NormalizedMaxPostsPerDay(); got != 3 {
			t.Errorf("got %d, want 3", got)
		}
	})
}

// --- MaxCharsForProvider ---

func TestMaxCharsForProvider(t *testing.T) {
	t.Parallel()
	cases := []struct {
		provider string
		override *int
		want     int
	}{
		{"mastodon", nil, 500},
		{"MASTODON", nil, 500},
		{"bluesky", nil, 300},
		{"BLUESKY", nil, 300},
		{"friendica", nil, 5000},
		{"unknown_provider", nil, 500},
		{"mastodon", intPtr(1000), 1000},
		{"bluesky", intPtr(280), 280},
		// override of 0 or negative is ignored
		{"mastodon", intPtr(0), 500},
		{"mastodon", intPtr(-1), 500},
	}
	for _, tc := range cases {
		got := MaxCharsForProvider(tc.provider, tc.override)
		if got != tc.want {
			t.Errorf("MaxCharsForProvider(%q, %v) = %d, want %d", tc.provider, tc.override, got, tc.want)
		}
	}
}

func intPtr(n int) *int { return &n }

// --- ResolveAutomationPostTitle ---

func TestResolveAutomationPostTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		templateTitle string
		aiTitle       string
		want          string
	}{
		{"template", "ai-generated", "ai-generated"},
		{"template", "  ", "template"},
		{"template", "", "template"},
		{"  template  ", "", "template"},
		{"", "ai", "ai"},
		{"", "", ""},
	}
	for _, tc := range cases {
		got := ResolveAutomationPostTitle(tc.templateTitle, tc.aiTitle)
		if got != tc.want {
			t.Errorf("ResolveAutomationPostTitle(%q, %q) = %q, want %q", tc.templateTitle, tc.aiTitle, got, tc.want)
		}
	}
}

// --- ExpandPostTemplateTitle ---

func TestExpandPostTemplateTitle(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)

	t.Run("counter substituted", func(t *testing.T) {
		got := ExpandPostTemplateTitle("Episode {counter}", t0, 7, nil, nil)
		if got != "Episode 7" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("date substituted", func(t *testing.T) {
		got := ExpandPostTemplateTitle("{year}-{month}-{day}", t0, 1, nil, nil)
		if got != "2026-03-15" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("result is trimmed", func(t *testing.T) {
		got := ExpandPostTemplateTitle("  static  ", t0, 0, nil, nil)
		if got != "static" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("empty template stays empty", func(t *testing.T) {
		got := ExpandPostTemplateTitle("", t0, 0, nil, nil)
		if got != "" {
			t.Errorf("got %q", got)
		}
	})
}

// --- BuildHashtagInsights ---

func TestBuildHashtagInsights(t *testing.T) {
	t.Parallel()

	t.Run("all zeros", func(t *testing.T) {
		ins := BuildHashtagInsights(0, 0, 0, 0, 0, 0)
		if ins.AvgTagsPerPost != 0 || ins.AvgEngagementWithTags != 0 || ins.AvgEngagementWithoutTags != 0 {
			t.Errorf("unexpected non-zero averages: %+v", ins)
		}
	})

	t.Run("averages computed", func(t *testing.T) {
		// 10 posts total, 4 with tags, 4 tag uses, engWithTags=40, engWithout=18
		ins := BuildHashtagInsights(10, 4, 2, 4, 40, 18)
		if ins.PostsTotal != 10 || ins.PostsWithTags != 4 {
			t.Fatalf("counts wrong: %+v", ins)
		}
		// avg_tags_per_post = 4/10 = 0.4
		if ins.AvgTagsPerPost != 0.4 {
			t.Errorf("AvgTagsPerPost = %f, want 0.4", ins.AvgTagsPerPost)
		}
		// avg_engagement_with_tags = 40/4 = 10
		if ins.AvgEngagementWithTags != 10.0 {
			t.Errorf("AvgEngagementWithTags = %f, want 10.0", ins.AvgEngagementWithTags)
		}
		// avg_engagement_without_tags = 18/(10-4) = 3
		if ins.AvgEngagementWithoutTags != 3.0 {
			t.Errorf("AvgEngagementWithoutTags = %f, want 3.0", ins.AvgEngagementWithoutTags)
		}
	})

	t.Run("all posts have tags", func(t *testing.T) {
		ins := BuildHashtagInsights(5, 5, 10, 20, 100, 0)
		// without = 0, so AvgEngagementWithoutTags stays 0
		if ins.AvgEngagementWithoutTags != 0 {
			t.Errorf("expected 0 for no-tag posts, got %f", ins.AvgEngagementWithoutTags)
		}
	})
}

// --- HashtagPerformance.FinalizeScores ---

func TestHashtagPerformance_FinalizeScores(t *testing.T) {
	t.Parallel()

	t.Run("normal case", func(t *testing.T) {
		h := &HashtagPerformance{Tag: "#go", Uses: 4, TotalEngagement: 20}
		h.FinalizeScores()
		// avg = 20/4 = 5; score = 20/(4+3) ≈ 2.857
		if h.AvgEngagement != 5.0 {
			t.Errorf("AvgEngagement = %f, want 5.0", h.AvgEngagement)
		}
		want := 20.0 / 7.0
		if h.Score != want {
			t.Errorf("Score = %f, want %f", h.Score, want)
		}
	})

	t.Run("zero uses", func(t *testing.T) {
		h := &HashtagPerformance{Tag: "#go", Uses: 0, TotalEngagement: 0}
		h.FinalizeScores()
		if h.AvgEngagement != 0 {
			t.Errorf("AvgEngagement should be 0 for zero uses, got %f", h.AvgEngagement)
		}
		// score = 0/(0+3) = 0
		if h.Score != 0 {
			t.Errorf("Score should be 0, got %f", h.Score)
		}
	})
}

// --- APITokenExpired ---

func TestAPITokenExpired(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("nil expiry never expires", func(t *testing.T) {
		if APITokenExpired(nil, now) {
			t.Error("nil expiry should not be expired")
		}
	})
	t.Run("future expiry is not expired", func(t *testing.T) {
		exp := now.Add(time.Hour)
		if APITokenExpired(&exp, now) {
			t.Error("future expiry should not be expired")
		}
	})
	t.Run("past expiry is expired", func(t *testing.T) {
		exp := now.Add(-time.Second)
		if !APITokenExpired(&exp, now) {
			t.Error("past expiry should be expired")
		}
	})
	t.Run("exact now is expired (not After)", func(t *testing.T) {
		exp := now
		if !APITokenExpired(&exp, now) {
			t.Error("expiry == now should be expired (not strictly after)")
		}
	})
}

// --- CreatePostInput.EffectiveContent ---

func TestCreatePostInput_EffectiveContent(t *testing.T) {
	t.Parallel()

	t.Run("returns override when present", func(t *testing.T) {
		in := CreatePostInput{
			Content:                "default",
			AccountContentOverride: map[string]string{"acc1": "override"},
		}
		if got := in.EffectiveContent("acc1"); got != "override" {
			t.Errorf("got %q, want override", got)
		}
	})
	t.Run("falls back to content when override empty", func(t *testing.T) {
		in := CreatePostInput{
			Content:                "default",
			AccountContentOverride: map[string]string{"acc1": "   "},
		}
		if got := in.EffectiveContent("acc1"); got != "default" {
			t.Errorf("got %q, want default", got)
		}
	})
	t.Run("falls back to content when account not in map", func(t *testing.T) {
		in := CreatePostInput{Content: "default"}
		if got := in.EffectiveContent("other"); got != "default" {
			t.Errorf("got %q, want default", got)
		}
	})
	t.Run("nil override map returns content", func(t *testing.T) {
		in := CreatePostInput{Content: "default", AccountContentOverride: nil}
		if got := in.EffectiveContent("acc1"); got != "default" {
			t.Errorf("got %q, want default", got)
		}
	})
}

// --- Validate methods ---

func TestKnowledgeSource_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		src     KnowledgeSource
		wantErr bool
	}{
		{
			name:    "valid text source",
			src:     KnowledgeSource{TeamID: "t1", Name: "kb", Type: KnowledgeSourceText, Content: "body"},
			wantErr: false,
		},
		{
			name:    "valid url source",
			src:     KnowledgeSource{TeamID: "t1", Name: "kb", Type: KnowledgeSourceURL, SourceURL: "https://example.com"},
			wantErr: false,
		},
		{
			name:    "valid file source",
			src:     KnowledgeSource{TeamID: "t1", Name: "kb", Type: KnowledgeSourceFile, Content: "data"},
			wantErr: false,
		},
		{
			name:    "missing team_id",
			src:     KnowledgeSource{Name: "kb", Type: KnowledgeSourceText, Content: "body"},
			wantErr: true,
		},
		{
			name:    "missing name",
			src:     KnowledgeSource{TeamID: "t1", Type: KnowledgeSourceText, Content: "body"},
			wantErr: true,
		},
		{
			name:    "invalid type",
			src:     KnowledgeSource{TeamID: "t1", Name: "kb", Type: "audio", Content: "body"},
			wantErr: true,
		},
		{
			name:    "text type missing content",
			src:     KnowledgeSource{TeamID: "t1", Name: "kb", Type: KnowledgeSourceText},
			wantErr: true,
		},
		{
			name:    "url type missing source_url",
			src:     KnowledgeSource{TeamID: "t1", Name: "kb", Type: KnowledgeSourceURL},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.src.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCampaignFormat_Validate(t *testing.T) {
	t.Parallel()
	structure := json.RawMessage(`{"foo":"bar"}`)
	cases := []struct {
		name    string
		cf      CampaignFormat
		wantErr bool
	}{
		{
			name:    "valid",
			cf:      CampaignFormat{TeamID: "t1", Name: "weekly", Structure: structure},
			wantErr: false,
		},
		{
			name:    "missing team_id",
			cf:      CampaignFormat{Name: "weekly", Structure: structure},
			wantErr: true,
		},
		{
			name:    "missing name",
			cf:      CampaignFormat{TeamID: "t1", Structure: structure},
			wantErr: true,
		},
		{
			name:    "missing structure",
			cf:      CampaignFormat{TeamID: "t1", Name: "weekly"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cf.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStyleExample_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		se      StyleExample
		wantErr bool
	}{
		{
			name:    "valid",
			se:      StyleExample{TeamID: "t1", Platform: "mastodon", Content: "hello"},
			wantErr: false,
		},
		{
			name:    "missing team_id",
			se:      StyleExample{Platform: "mastodon", Content: "hello"},
			wantErr: true,
		},
		{
			name:    "missing platform",
			se:      StyleExample{TeamID: "t1", Content: "hello"},
			wantErr: true,
		},
		{
			name:    "missing content",
			se:      StyleExample{TeamID: "t1", Platform: "mastodon"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.se.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- ResolvePostStatusOnUpdate ---

func TestResolvePostStatusOnUpdate(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-time.Hour)

	cases := []struct {
		name   string
		was    PostStatus
		source PostSource
		in     CreatePostInput
		want   PostStatus
	}{
		{
			name:   "imported always returns posted",
			was:    PostStatusDraft,
			source: PostSourceImported,
			in:     CreatePostInput{ScheduledAt: past},
			want:   PostStatusPosted,
		},
		{
			name:   "already posted stays posted",
			was:    PostStatusPosted,
			source: PostSourceScheduled,
			in:     CreatePostInput{ScheduledAt: future},
			want:   PostStatusPosted,
		},
		{
			name:   "processing stays processing",
			was:    PostStatusProcessing,
			source: PostSourceScheduled,
			in:     CreatePostInput{ScheduledAt: future},
			want:   PostStatusProcessing,
		},
		{
			name:   "cancelled stays cancelled",
			was:    PostStatusCancelled,
			source: PostSourceScheduled,
			in:     CreatePostInput{ScheduledAt: future},
			want:   PostStatusCancelled,
		},
		{
			name:   "future non-draft becomes pending",
			was:    PostStatusFailed,
			source: PostSourceScheduled,
			in:     CreatePostInput{ScheduledAt: future, Draft: false},
			want:   PostStatusPending,
		},
		{
			name:   "explicit draft becomes draft",
			was:    PostStatusPending,
			source: PostSourceScheduled,
			in:     CreatePostInput{ScheduledAt: future, Draft: true},
			want:   PostStatusDraft,
		},
		{
			name:   "past draft-was-status with non-draft in stays as was",
			was:    PostStatusFailed,
			source: PostSourceScheduled,
			in:     CreatePostInput{ScheduledAt: past, Draft: false},
			want:   PostStatusFailed,
		},
		{
			name:   "draft was status promoted to pending when non-draft past",
			was:    PostStatusDraft,
			source: PostSourceScheduled,
			in:     CreatePostInput{ScheduledAt: past, Draft: false},
			want:   PostStatusPending,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolvePostStatusOnUpdate(tc.was, tc.source, tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// --- PostPatchFieldsSet.Any ---

func TestPostPatchFieldsSet_Any(t *testing.T) {
	t.Parallel()

	t.Run("empty returns false", func(t *testing.T) {
		if (PostPatchFieldsSet{}).Any() {
			t.Error("empty set must return false")
		}
	})
	t.Run("Title returns true", func(t *testing.T) {
		if !(PostPatchFieldsSet{Title: true}).Any() {
			t.Error("expected true for Title=true")
		}
	})
	t.Run("Content returns true", func(t *testing.T) {
		if !(PostPatchFieldsSet{Content: true}).Any() {
			t.Error("expected true")
		}
	})
	t.Run("all fields false returns false", func(t *testing.T) {
		f := PostPatchFieldsSet{
			Title: false, Content: false, ScheduledAt: false,
			TargetAccounts: false, Visibility: false,
			MediaIDs: false, MediaExcludeByAccount: false,
			Versions: false, Draft: false,
		}
		if f.Any() {
			t.Error("all false must return false")
		}
	})
	t.Run("Draft returns true", func(t *testing.T) {
		if !(PostPatchFieldsSet{Draft: true}).Any() {
			t.Error("expected true")
		}
	})
	t.Run("MediaExcludeByAccount returns true", func(t *testing.T) {
		if !(PostPatchFieldsSet{MediaExcludeByAccount: true}).Any() {
			t.Error("expected true")
		}
	})
}

// --- NewReviewQueueItem ---

func TestNewReviewQueueItem(t *testing.T) {
	t.Parallel()

	past := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	future := past.Add(48 * time.Hour)
	now := past.Add(24 * time.Hour)

	t.Run("overdue draft", func(t *testing.T) {
		post := ScheduledPost{
			ID:          "p1",
			Status:      PostStatusDraft,
			ScheduledAt: past,
		}
		item := NewReviewQueueItem(post, "my-feed", now)
		if !item.IsOverdue {
			t.Error("should be overdue")
		}
		if item.RSSFeedName != "my-feed" {
			t.Errorf("feed name = %q", item.RSSFeedName)
		}
	})

	t.Run("not overdue when scheduled in future", func(t *testing.T) {
		post := ScheduledPost{
			ID:          "p2",
			Status:      PostStatusDraft,
			ScheduledAt: future,
		}
		item := NewReviewQueueItem(post, "", now)
		if item.IsOverdue {
			t.Error("future draft should not be overdue")
		}
	})

	t.Run("non-draft is never overdue", func(t *testing.T) {
		post := ScheduledPost{
			ID:          "p3",
			Status:      PostStatusPending,
			ScheduledAt: past,
		}
		item := NewReviewQueueItem(post, "", now)
		if item.IsOverdue {
			t.Error("non-draft should not be overdue regardless of time")
		}
	})

	t.Run("zero now falls back to real time", func(t *testing.T) {
		// Scheduled far in the future so it's never overdue even with real now.
		post := ScheduledPost{
			ID:          "p4",
			Status:      PostStatusDraft,
			ScheduledAt: time.Now().Add(365 * 24 * time.Hour),
		}
		item := NewReviewQueueItem(post, "", time.Time{})
		if item.IsOverdue {
			t.Error("far-future draft should not be overdue")
		}
	})
}
