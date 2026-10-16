package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"git.f4mily.net/goloom/internal/domain"
)

// Limits for team AI settings input. They bound prompt size and storage per
// team; handlers reject violations with a 400 and a translated error code.
const (
	maxProfileTonalityLen    = 200
	maxProfileLanguageLen    = 35
	maxProfileListItems      = 200 // banned words
	maxProfileRuleItems      = 50  // formatting rules
	maxProfileListItemLen    = 500
	maxProfileHashtags       = 30
	maxFormatNameLen         = 120
	maxFormatStructureBytes  = 16 * 1024
	maxFormatHashtags        = 30
	maxHashtagLen            = 100
	maxStyleContentLen       = 5000
	maxStylePlatformLen      = 50
	maxStyleNotesLen         = 1000
	maxKnowledgeNameLen      = 200
	maxKnowledgeURLLen       = 2048
	maxKnowledgeContentChars = 20_000 // characters per source; matches the AI prompt budget
	maxKnowledgeSources      = 50     // per team
	maxProactiveGapDays      = 365
	maxProactiveTriggers     = 100
	maxCronScheduleLen       = 100
)

var errInvalidInput = errors.New("invalid input")

func tooLong(s string, max int) bool { return utf8.RuneCountInString(s) > max }

func validateTeamProfile(p domain.TeamProfile) error {
	m := p.StyleMetadata
	if m.MaxHashtags < 0 || m.MaxHashtags > maxProfileHashtags {
		return errInvalidInput
	}
	if tooLong(m.Tonality, maxProfileTonalityLen) || tooLong(m.PreferredLanguage, maxProfileLanguageLen) {
		return errInvalidInput
	}
	if len(m.BannedWords) > maxProfileListItems || len(m.FormattingRules) > maxProfileRuleItems {
		return errInvalidInput
	}
	for _, list := range [][]string{m.BannedWords, m.FormattingRules} {
		for _, item := range list {
			if tooLong(item, maxProfileListItemLen) {
				return errInvalidInput
			}
		}
	}
	return nil
}

func validateCampaignFormat(f domain.CampaignFormat) error {
	name := strings.TrimSpace(f.Name)
	if name == "" || tooLong(name, maxFormatNameLen) {
		return errInvalidInput
	}
	if f.Weekday != nil && (*f.Weekday < 0 || *f.Weekday > 6) {
		return errInvalidInput
	}
	if len(f.Structure) == 0 || len(f.Structure) > maxFormatStructureBytes {
		return errInvalidInput
	}
	if len(f.RequiredHashtags) > maxFormatHashtags {
		return errInvalidInput
	}
	for _, h := range f.RequiredHashtags {
		if tooLong(h, maxHashtagLen) {
			return errInvalidInput
		}
	}
	return nil
}

func validateStyleExample(e domain.StyleExample) error {
	if strings.TrimSpace(e.Content) == "" || tooLong(e.Content, maxStyleContentLen) {
		return errInvalidInput
	}
	if tooLong(e.Platform, maxStylePlatformLen) || tooLong(e.Notes, maxStyleNotesLen) {
		return errInvalidInput
	}
	return nil
}

// validateKnowledgeSource checks shape and field lengths; the content size cap
// is separate (knowledge_source_too_large) because URL sources fetch content
// after the first validation pass.
func validateKnowledgeSource(k domain.KnowledgeSource) error {
	if err := k.Validate(); err != nil {
		return err
	}
	if tooLong(k.Name, maxKnowledgeNameLen) || len(k.SourceURL) > maxKnowledgeURLLen {
		return errInvalidInput
	}
	return nil
}

func validateProactiveSettings(p domain.ProactiveTriggerSettings) error {
	if p.ContentGapThresholdDays < 1 || p.ContentGapThresholdDays > maxProactiveGapDays {
		return errInvalidInput
	}
	if p.MaxTriggersPerDay < 1 || p.MaxTriggersPerDay > maxProactiveTriggers {
		return errInvalidInput
	}
	cron := strings.TrimSpace(p.CronSchedule)
	if cron == "" || len(cron) > maxCronScheduleLen || len(strings.Fields(cron)) != 5 {
		return errInvalidInput
	}
	return nil
}

// writeStoreError maps a store failure to a response: domain.ErrNotFound gives
// 404 with notFoundKey, everything else is logged and answered with a generic
// 500 so internal error text never reaches the client.
func (a *API) writeStoreError(w http.ResponseWriter, r *http.Request, err error, notFoundKey string) {
	if notFoundKey != "" && errors.Is(err, domain.ErrNotFound) {
		a.writeError(w, r, notFoundKey, http.StatusNotFound)
		return
	}
	a.log.Error("team ai settings store error", "path", r.URL.Path, "error", err)
	a.writeError(w, r, "internal_error", http.StatusInternalServerError)
}
