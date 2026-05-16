package provider

import "testing"

func TestParseBlueskyGetPostsResponse_postView(t *testing.T) {
	raw := []byte(`{"posts":[{"uri":"at://did:plc:x/app.bsky.feed.post/abc","likeCount":2,"repostCount":1,"replyCount":0,"quoteCount":0}]}`)
	counts, err := parseBlueskyGetPostsResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if counts.LikeCount != 2 || counts.RepostCount != 1 {
		t.Fatalf("counts: %#v", counts)
	}
}

func TestParseBlueskyGetPostsResponse_feedViewPost(t *testing.T) {
	raw := []byte(`{"posts":[{"post":{"likeCount":7,"repostCount":0,"replyCount":1,"quoteCount":0}}]}`)
	counts, err := parseBlueskyGetPostsResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if counts.LikeCount != 7 {
		t.Fatalf("likeCount: %d", counts.LikeCount)
	}
}

func TestParseBlueskyGetPostsResponse_countsOnOuterWithNestedPost(t *testing.T) {
	raw := []byte(`{"posts":[{"likeCount":2,"repostCount":1,"post":{"uri":"at://did:plc:x/app.bsky.feed.post/abc","record":{"$type":"app.bsky.feed.post"}}}]}`)
	counts, err := parseBlueskyGetPostsResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if counts.LikeCount != 2 || counts.RepostCount != 1 {
		t.Fatalf("counts: %#v", counts)
	}
}
