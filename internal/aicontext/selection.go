package aicontext

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

// ErrKnowledgeSourceUnavailable is deliberately nonspecific: deleted and foreign
// IDs have identical behavior. Never substitute team defaults for missing IDs.
var ErrKnowledgeSourceUnavailable = errors.New("knowledge_source_unavailable")

// Select applies saved campaign or explicit RSS knowledge selection and target
// platform preferences. Generation and previews must call this before prompting.
// The input is not mutated. All source records are checked against the active team.
func Select(in domain.AIContext, raw json.RawMessage) (domain.AIContext, error) {
	var p struct {
		KnowledgeSourceIDs  []string `json:"knowledge_source_ids"`
		CampaignFormatID    string   `json:"campaign_format_id"`
		CampaignFormatAlias string   `json:"campaignFormatId"`
		TargetAccountIDs    []string `json:"target_account_ids"`
		TargetAccountAlias  []string `json:"targetAccountIds"`
		Platform            string   `json:"platform"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return domain.AIContext{}, err
		}
	}
	ids := p.KnowledgeSourceIDs
	formatID := p.CampaignFormatID
	if formatID == "" {
		formatID = p.CampaignFormatAlias
	}
	if formatID != "" {
		found := false
		for _, f := range in.CampaignFormats {
			if f.ID == formatID && f.TeamID == in.Team.ID {
				ids = f.KnowledgeSourceIDs
				found = true
				break
			}
		}
		if !found {
			return domain.AIContext{}, ErrKnowledgeSourceUnavailable
		}
	}
	if len(ids) > 50 {
		return domain.AIContext{}, ErrKnowledgeSourceUnavailable
	}
	available := map[string]domain.KnowledgeSource{}
	out := in
	out.KnowledgeSources = []domain.KnowledgeSource{}
	for _, s := range in.KnowledgeSources {
		if s.TeamID == in.Team.ID {
			available[s.ID] = s
			if ids == nil {
				out.KnowledgeSources = append(out.KnowledgeSources, s)
			}
		}
	}
	if ids != nil {
		seen := map[string]bool{}
		for _, id := range ids {
			s, ok := available[id]
			if !ok {
				return domain.AIContext{}, ErrKnowledgeSourceUnavailable
			}
			if !seen[id] {
				out.KnowledgeSources = append(out.KnowledgeSources, s)
				seen[id] = true
			}
		}
	}
	platforms := map[string]bool{}
	targets := p.TargetAccountIDs
	if len(targets) == 0 {
		targets = p.TargetAccountAlias
	}
	for _, id := range targets {
		for _, account := range in.Accounts {
			if account.ID == id {
				platforms[strings.ToLower(string(account.Provider))] = true
			}
		}
	}
	if len(platforms) == 0 && p.Platform != "" {
		platforms[strings.ToLower(p.Platform)] = true
	}
	examples := append([]domain.StyleExample{}, in.StyleExamples...)
	sort.SliceStable(examples, func(i, j int) bool { return examples[i].CreatedAt.After(examples[j].CreatedAt) })
	out.StyleExamples = []domain.StyleExample{}
	if len(platforms) == 0 {
		for _, ex := range examples {
			if ex.TeamID == in.Team.ID {
				out.StyleExamples = append(out.StyleExamples, ex)
			}
		}
		return out, nil
	}
	for _, general := range []bool{false, true} {
		for _, ex := range examples {
			platform := strings.ToLower(strings.TrimSpace(ex.Platform))
			isGeneral := platform == "" || platform == "general" || platform == "all"
			if ex.TeamID != in.Team.ID || strings.TrimSpace(ex.Content) == "" || isGeneral != general || (!isGeneral && !platforms[platform]) {
				continue
			}
			out.StyleExamples = append(out.StyleExamples, ex)
			if len(out.StyleExamples) == 3 {
				return out, nil
			}
		}
	}
	return out, nil
}
