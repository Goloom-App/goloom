package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryDocumentsPerAccountVersions(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/discovery", nil)
	(&API{}).handleDiscovery(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var doc map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	schemas, _ := doc["schemas"].(map[string]any)
	if schemas == nil {
		t.Fatal("missing schemas")
	}
	createPost, _ := schemas["CreatePostInput"].(map[string]any)
	if createPost == nil {
		t.Fatal("missing schemas.CreatePostInput")
	}
	props, _ := createPost["properties"].(map[string]any)
	if props["account_content_override"] == nil {
		t.Error("CreatePostInput missing account_content_override")
	}
	if props["use_versions"] == nil {
		t.Error("CreatePostInput missing use_versions")
	}

	examples, _ := doc["examples"].(map[string]any)
	if examples["create_post_with_per_account_text"] == nil {
		t.Error("missing example create_post_with_per_account_text")
	}

	endpoints, _ := doc["endpoints"].([]any)
	var hasVersionsPatch bool
	for _, raw := range endpoints {
		ep, _ := raw.(map[string]any)
		if ep["path"] == "/v1/teams/{teamID}/posts/{postID}/versions" && ep["method"] == "PATCH" {
			hasVersionsPatch = true
			rb, _ := ep["request_body"].(map[string]any)
			if rb["schema"] != "PatchPostVersionsInput" {
				t.Errorf("versions PATCH request_body.schema = %v", rb["schema"])
			}
		}
	}
	if !hasVersionsPatch {
		t.Error("missing PATCH /v1/teams/{teamID}/posts/{postID}/versions endpoint")
	}

	guides, _ := doc["agent_guides"].([]any)
	if len(guides) == 0 {
		t.Error("missing agent_guides")
	}
}
