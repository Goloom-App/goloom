package api

import (
	"context"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
)

// validatePostInput is a test-only shim preserving the pre-pipeline entry point:
// it runs the shared post pipeline and maps the result onto the REST validation
// DTO, so the existing validation tests keep exercising the REST-facing shape.
func (a *API) validatePostInput(ctx context.Context, teamID string, input domain.CreatePostInput) (validationResponse, string, error) {
	prepared, err := a.posts.Prepare(ctx, teamID, input, postservice.Options{CheckLimits: !input.Draft})
	if err != nil {
		return validationResponse{}, "", err
	}
	return toValidationResponse(prepared.Validation), prepared.EffectiveTeam, nil
}
