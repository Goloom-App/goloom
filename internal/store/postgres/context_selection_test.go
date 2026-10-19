package postgres_test

import (
	"context"
	"encoding/json"
	"git.f4mily.net/goloom/internal/domain"
	"reflect"
	"testing"
)

func TestKnowledgeSelectionPersistence(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)
	for _, ids := range [][]string{nil, {}, {"old", "new"}} {
		f, err := s.CreateCampaignFormat(ctx, team.ID, domain.CampaignFormat{Name: "selected", Structure: json.RawMessage(`{}`), KnowledgeSourceIDs: ids})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(f.KnowledgeSourceIDs, ids) {
			t.Fatalf("campaign create: %#v want %#v", f.KnowledgeSourceIDs, ids)
		}
		f.KnowledgeSourceIDs = []string{"changed"}
		f, err = s.UpdateCampaignFormat(ctx, team.ID, f.ID, f)
		if err != nil {
			t.Fatal(err)
		}
		f.KnowledgeSourceIDs = ids
		f, err = s.UpdateCampaignFormat(ctx, team.ID, f.ID, f)
		if err != nil {
			t.Fatal(err)
		}
		list, err := s.ListCampaignFormats(ctx, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range list {
			if v.ID == f.ID && !reflect.DeepEqual(v.KnowledgeSourceIDs, ids) {
				t.Fatalf("campaign list: %#v", v.KnowledgeSourceIDs)
			}
		}
		feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.org/feed", Name: "selected", KnowledgeSourceIDs: ids})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(feed.KnowledgeSourceIDs, ids) {
			t.Fatalf("feed create: %#v want %#v", feed.KnowledgeSourceIDs, ids)
		}
		feed.KnowledgeSourceIDs = []string{"changed"}
		feed, err = s.UpdateRSSFeedConfig(ctx, team.ID, feed.ID, feed)
		if err != nil {
			t.Fatal(err)
		}
		feed.KnowledgeSourceIDs = ids
		feed, err = s.UpdateRSSFeedConfig(ctx, team.ID, feed.ID, feed)
		if err != nil {
			t.Fatal(err)
		}
		feeds, err := s.ListRSSFeedConfigs(ctx, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range feeds {
			if v.ID == feed.ID && !reflect.DeepEqual(v.KnowledgeSourceIDs, ids) {
				t.Fatalf("feed list: %#v", v.KnowledgeSourceIDs)
			}
		}
	}
}
