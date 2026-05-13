package api

import (
	"encoding/json"
	"net/http"
)

// handleDiscovery returns a machine-readable overview of the API endpoints.
// This is designed to help AI agents understand the available capabilities.
func (a *API) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	type parameter struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Required    bool   `json:"required"`
	}

	type endpoint struct {
		Path        string      `json:"path"`
		Method      string      `json:"method"`
		Description string      `json:"description"`
		Parameters  []parameter `json:"parameters,omitempty"`
		Auth        string      `json:"auth_type"`
	}

	discovery := map[string]any{
		"app_name":    "Goloom",
		"version":     "v1",
		"description": "Social Media Scheduling & Analytics Platform",
		"endpoints": []endpoint{
			{
				Path:        "/v1/discovery",
				Method:      "GET",
				Description: "Retrieve this API discovery document",
				Auth:        "none",
			},
			{
				Path:        "/v1/teams",
				Method:      "GET",
				Description: "List all teams the current user belongs to",
				Auth:        "bearer_token",
			},
			{
				Path:        "/v1/teams/{teamID}/posts",
				Method:      "GET",
				Description: "List all scheduled and posted items for a team",
				Parameters: []parameter{
					{Name: "teamID", Type: "uuid", Description: "The unique identifier of the team", Required: true},
				},
				Auth: "bearer_token",
			},
			{
				Path:        "/v1/teams/{teamID}/posts",
				Method:      "POST",
				Description: "Create a new scheduled post or draft",
				Parameters: []parameter{
					{Name: "teamID", Type: "uuid", Description: "The unique identifier of the team", Required: true},
				},
				Auth: "bearer_token",
			},
			{
				Path:        "/v1/teams/{teamID}/posts/{postID}",
				Method:      "PATCH",
				Description: "Update an existing post (content, schedule, media)",
				Parameters: []parameter{
					{Name: "teamID", Type: "uuid", Description: "The team ID", Required: true},
					{Name: "postID", Type: "uuid", Description: "The post ID", Required: true},
				},
				Auth: "bearer_token",
			},
			{
				Path:        "/v1/teams/{teamID}/accounts",
				Method:      "GET",
				Description: "List connected social media accounts for a team",
				Auth:        "bearer_token",
			},
			{
				Path:        "/v1/teams/{teamID}/analytics/summary",
				Method:      "GET",
				Description: "Get high-level engagement and follower growth metrics",
				Auth:        "bearer_token",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(discovery)
}
