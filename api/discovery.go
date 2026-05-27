package api

import (
	"encoding/json"
	"net/http"
)

// discovery types are structured for AI agents: paths, auth, request schemas, workflows, examples.

type discoveryParameter struct {
	Name        string `json:"name"`
	In          string `json:"in"` // path | body
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type discoveryProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Items       string `json:"items,omitempty"`       // element type for arrays
	Additional  string `json:"value_type,omitempty"` // value type for maps
}

type discoverySchema struct {
	Description string                       `json:"description,omitempty"`
	Required    []string                     `json:"required,omitempty"`
	Properties  map[string]discoveryProperty `json:"properties,omitempty"`
}

type discoveryRequestBody struct {
	ContentType string          `json:"content_type"`
	Schema      string          `json:"schema"` // key in top-level schemas map
	Description string          `json:"description,omitempty"`
}

type discoveryEndpoint struct {
	Path        string                `json:"path"`
	Method      string                `json:"method"`
	Description string                `json:"description"`
	Auth        string                `json:"auth_type"`
	Parameters  []discoveryParameter  `json:"parameters,omitempty"`
	RequestBody *discoveryRequestBody `json:"request_body,omitempty"`
	Response    string                `json:"response_description,omitempty"`
}

type discoveryGuide struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Steps       []string `json:"steps"`
	ExampleKey  string   `json:"example_key,omitempty"`
	Related     []string `json:"related_schemas,omitempty"`
}

type discoveryDocument struct {
	AppName          string                       `json:"app_name"`
	Version          string                       `json:"version"`
	Description      string                       `json:"description"`
	OpenAPISpecNote  string                       `json:"openapi_spec_note"`
	Auth             map[string]string            `json:"auth"`
	Schemas          map[string]discoverySchema   `json:"schemas"`
	AgentGuides      []discoveryGuide             `json:"agent_guides"`
	Examples         map[string]any               `json:"examples"`
	Endpoints        []discoveryEndpoint          `json:"endpoints"`
}

func buildDiscoveryDocument() discoveryDocument {
	schemas := map[string]discoverySchema{
		"CreatePostInput": {
			Description: "Body for POST /posts and POST .../posts/validate. Default content applies to all target_accounts unless overridden per account.",
			Required:    []string{"content"},
			Properties: map[string]discoveryProperty{
				"title":                    {Type: "string", Description: "Internal title (optional)"},
				"content":                  {Type: "string", Description: "Default message for all target accounts"},
				"scheduled_at":             {Type: "string", Description: "RFC3339 UTC datetime; defaults to now if omitted on create"},
				"target_accounts":          {Type: "array", Items: "uuid", Description: "Social account IDs to publish to; required unless draft=true"},
				"visibility":               {Type: "string", Description: "e.g. public (provider-specific)"},
				"draft":                    {Type: "boolean", Description: "If true, saves as draft; target_accounts optional"},
				"media_ids":                {Type: "array", Items: "string", Description: "Library media IDs attached to the post"},
				"media_exclude_by_account": {Type: "object", Additional: "array of media_id strings", Description: "Per-account media exclusions"},
				"account_content_override": {
					Type:        "object",
					Additional:  "string",
					Description: "Map account_id (uuid) -> post text. Keys MUST be included in target_accounts. Empty values are ignored; missing keys use content.",
				},
				"use_versions": {
					Type:        "boolean",
					Description: "When true, character-limit validation uses each account's override (or content) separately instead of failing on the default content length.",
				},
			},
		},
		"UpdatePostPatch": {
			Description: "Partial update for PATCH /posts/{postID}. Only include fields to change. Omitted fields stay as stored (including post_versions).",
			Properties: map[string]discoveryProperty{
				"title":                    {Type: "string"},
				"content":                  {Type: "string"},
				"scheduled_at":             {Type: "string", Description: "RFC3339 UTC datetime"},
				"target_accounts":          {Type: "array", Items: "uuid"},
				"visibility":               {Type: "string"},
				"draft":                    {Type: "boolean"},
				"media_ids":                {Type: "array", Items: "string"},
				"media_exclude_by_account": {Type: "object", Additional: "array of media_id strings"},
				"account_content_override": {
					Type:        "object",
					Additional:  "string",
					Description: "When present, replaces all per-account overrides. When omitted, existing overrides are kept.",
				},
			},
		},
		"PatchPostVersionsInput": {
			Description: "Replace or upsert per-account text overrides for an existing post. Each account_id must already be in the post's target_accounts.",
			Required:    []string{"versions"},
			Properties: map[string]discoveryProperty{
				"versions": {
					Type:        "array",
					Items:       "PostVersionRow",
					Description: "List of {account_id, content}. Empty content deletes the override for that account.",
				},
			},
		},
		"PostVersionRow": {
			Required: []string{"account_id", "content"},
			Properties: map[string]discoveryProperty{
				"account_id": {Type: "string", Description: "UUID of a targeted social account"},
				"content":    {Type: "string", Description: "Override text for that account"},
			},
		},
		"PostVersion": {
			Description: "Stored per-account override returned by versions list/patch.",
			Properties: map[string]discoveryProperty{
				"post_id":    {Type: "string", Description: "Post UUID"},
				"account_id": {Type: "string", Description: "Account UUID"},
				"content":    {Type: "string", Description: "Override text"},
			},
		},
	}

	examples := map[string]any{
		"create_post_with_per_account_text": map[string]any{
			"method": "POST",
			"path":   "/v1/teams/{teamID}/posts",
			"headers": map[string]string{
				"Authorization": "Bearer <token>",
				"Content-Type":  "application/json",
			},
			"body": map[string]any{
				"title":           "Cross-post",
				"content":         "Default text for accounts without an override",
				"scheduled_at":    "2026-05-20T15:30:00Z",
				"target_accounts": []string{"<account-uuid-1>", "<account-uuid-2>"},
				"account_content_override": map[string]string{
					"<account-uuid-1>": "Shorter Mastodon-specific text",
					"<account-uuid-2>": "Bluesky-specific text",
				},
				"use_versions": true,
			},
			"notes": []string{
				"account_content_override keys must match entries in target_accounts",
				"Prefer this on create/update; alternative is PATCH .../posts/{postID}/versions after the post exists",
			},
		},
		"patch_post_versions": map[string]any{
			"method": "PATCH",
			"path":   "/v1/teams/{teamID}/posts/{postID}/versions",
			"body": map[string]any{
				"versions": []map[string]string{
					{"account_id": "<account-uuid-1>", "content": "Text for account 1"},
					{"account_id": "<account-uuid-2>", "content": "Text for account 2"},
				},
			},
			"notes": []string{
				"Returns { items: PostVersion[] }",
				"400 account not targeted by post if account_id is not in the post's targets",
			},
		},
	}

	return discoveryDocument{
		AppName:         "Goloom",
		Version:         "v1",
		Description:     "Social Media Scheduling & Analytics Platform",
		OpenAPISpecNote: "Full OpenAPI 3.1 spec: docs/api/openapi.yaml in the repository (not served by default).",
		Auth: map[string]string{
			"type":        "bearer_token",
			"header":      "Authorization: Bearer <token>",
			"description": "API token from admin bootstrap or team member session",
		},
		Schemas: schemas,
		AgentGuides: []discoveryGuide{
			{
				ID:    "per_account_post_content",
				Title: "Different post text per destination account",
				Summary: "Use account_content_override on create/update, or PATCH post versions after creation. " +
					"Overrides are stored in post_versions and used at publish time per account.",
				Steps: []string{
					"GET /v1/teams — pick team id",
					"GET /v1/teams/{teamID}/accounts — list account ids and providers",
					"POST /v1/teams/{teamID}/posts with content, target_accounts, and account_content_override (see example create_post_with_per_account_text)",
					"Optional: POST /v1/teams/{teamID}/posts/validate with the same body to check per-account character limits",
					"Alternative after post exists: PATCH /v1/teams/{teamID}/posts/{postID}/versions (see example patch_post_versions)",
					"Read overrides: GET /v1/teams/{teamID}/posts/{postID}/versions",
				},
				ExampleKey: "create_post_with_per_account_text",
				Related:    []string{"CreatePostInput", "PatchPostVersionsInput", "PostVersion"},
			},
		},
		Examples: examples,
		Endpoints: []discoveryEndpoint{
			{
				Path: "/v1/discovery", Method: "GET",
				Description: "This document: endpoints, schemas, agent guides, and examples",
				Auth:        "none",
			},
			{
				Path: "/v1/teams", Method: "GET",
				Description: "List teams for the authenticated user. Response: { items: Team[] }",
				Auth:        "bearer_token",
			},
			{
				Path: "/v1/teams/{teamID}/accounts", Method: "GET",
				Description: "List connected social accounts (ids needed for target_accounts and overrides)",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Description: "Team id", Required: true},
				},
				Response: "{ items: SocialAccount[] } with id, provider, username",
			},
			{
				Path: "/v1/teams/{teamID}/posts", Method: "GET",
				Description: "List scheduled, draft, and posted items for a team",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
				},
				Response: "{ items: ScheduledPost[] }",
			},
			{
				Path: "/v1/teams/{teamID}/posts", Method: "POST",
				Description: "Create a scheduled post or draft. Set account_content_override for per-account text.",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
				},
				RequestBody: &discoveryRequestBody{
					ContentType: "application/json",
					Schema:      "CreatePostInput",
					Description: "See examples.create_post_with_per_account_text",
				},
				Response: "201 ScheduledPost; 422 validation body if limits exceeded (unless use_versions)",
			},
			{
				Path: "/v1/teams/{teamID}/posts/validate", Method: "POST",
				Description: "Validate post body against provider character limits per destination (uses overrides when present)",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
				},
				RequestBody: &discoveryRequestBody{
					ContentType: "application/json",
					Schema:      "CreatePostInput",
				},
				Response: "{ valid, max_chars, content_length, destinations: [{ account_id, provider, max_chars, length, valid }] }",
			},
			{
				Path: "/v1/teams/{teamID}/posts/{postID}", Method: "PATCH",
				Description: "Partial update: only sent fields change. Reschedule with {\"scheduled_at\": \"...\"} without touching text or overrides.",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
					{Name: "postID", In: "path", Type: "uuid", Required: true, Description: "Post id"},
				},
				RequestBody: &discoveryRequestBody{
					ContentType: "application/json",
					Schema:      "UpdatePostPatch",
				},
				Response: "200 ScheduledPost",
			},
			{
				Path: "/v1/teams/{teamID}/posts/{postID}/versions", Method: "GET",
				Description: "List per-account content overrides (post versions) for one post",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
					{Name: "postID", In: "path", Type: "uuid", Required: true, Description: "Post id"},
				},
				Response: "{ items: PostVersion[] }",
			},
			{
				Path: "/v1/teams/{teamID}/posts/{postID}/versions", Method: "PATCH",
				Description: "Upsert or delete per-account text overrides without replacing the whole post",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
					{Name: "postID", In: "path", Type: "uuid", Required: true, Description: "Post id"},
				},
				RequestBody: &discoveryRequestBody{
					ContentType: "application/json",
					Schema:      "PatchPostVersionsInput",
					Description: "See examples.patch_post_versions",
				},
				Response: "{ items: PostVersion[] }",
			},
			{
				Path: "/v1/teams/{teamID}/versions", Method: "GET",
				Description: "List all post versions (overrides) across every post in the team",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
				},
				Response: "{ items: PostVersion[] }",
			},
			{
				Path: "/v1/teams/{teamID}/analytics/summary", Method: "GET",
				Description: "High-level engagement and follower growth metrics",
				Auth:        "bearer_token",
				Parameters: []discoveryParameter{
					{Name: "teamID", In: "path", Type: "uuid", Required: true, Description: "Team id"},
				},
			},
		},
	}
}

// handleDiscovery returns a machine-readable overview of the API endpoints.
// This is designed to help AI agents understand the available capabilities.
func (a *API) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildDiscoveryDocument())
}
