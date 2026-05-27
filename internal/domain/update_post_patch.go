package domain

import (
	"strings"
	"time"
)

// UpdatePostPatch is the PATCH body for scheduled posts. Omitted keys are left unchanged.
type UpdatePostPatch struct {
	Title                  PatchField[string]
	Content                PatchField[string]
	ScheduledAt            PatchField[time.Time]
	TargetAccounts         PatchField[[]string]
	Visibility             PatchField[string]
	MediaIDs               PatchField[[]string]
	MediaExcludeByAccount  PatchField[map[string][]string]
	AccountContentOverride PatchField[map[string]string]
	Draft                  PatchField[bool]
}

// PostPatchFieldsSet records which logical groups should be written to storage.
type PostPatchFieldsSet struct {
	Title                 bool
	Content               bool
	ScheduledAt           bool
	TargetAccounts        bool
	Visibility            bool
	MediaIDs              bool
	MediaExcludeByAccount bool
	Versions              bool
	Draft                 bool
}

func (f PostPatchFieldsSet) Any() bool {
	return f.Title || f.Content || f.ScheduledAt || f.TargetAccounts || f.Visibility ||
		f.MediaIDs || f.MediaExcludeByAccount || f.Versions || f.Draft
}

// VersionsToOverrideMap builds the override map used for validation from stored versions.
func VersionsToOverrideMap(versions []PostVersion) map[string]string {
	out := make(map[string]string)
	for _, v := range versions {
		if strings.TrimSpace(v.Content) != "" {
			out[v.AccountID] = v.Content
		}
	}
	return out
}

// ApplyPostPatch merges a PATCH onto the stored post and versions for validation and persistence.
func ApplyPostPatch(existing ScheduledPost, versions []PostVersion, patch UpdatePostPatch) (CreatePostInput, PostPatchFieldsSet) {
	merged := CreatePostInput{
		Title:                 existing.Title,
		Content:               existing.Content,
		ScheduledAt:           existing.ScheduledAt,
		TargetAccounts:        append([]string(nil), existing.TargetAccounts...),
		Visibility:            existing.Visibility,
		MediaIDs:              append([]string(nil), existing.MediaIDs...),
		MediaExcludeByAccount: cloneMediaExclude(existing.MediaExcludeByAccount),
		Draft:                 existing.Status == PostStatusDraft,
	}
	var flags PostPatchFieldsSet

	if patch.Title.Set {
		merged.Title = strings.TrimSpace(patch.Title.Value)
		flags.Title = true
	}
	if patch.Content.Set {
		merged.Content = strings.TrimSpace(patch.Content.Value)
		flags.Content = true
	}
	if patch.ScheduledAt.Set {
		merged.ScheduledAt = patch.ScheduledAt.Value
		flags.ScheduledAt = true
	}
	if patch.TargetAccounts.Set {
		merged.TargetAccounts = append([]string(nil), patch.TargetAccounts.Value...)
		flags.TargetAccounts = true
	}
	if patch.Visibility.Set {
		merged.Visibility = NormalizePostVisibility(patch.Visibility.Value)
		flags.Visibility = true
	}
	if patch.MediaIDs.Set {
		merged.MediaIDs = NormalizeMediaIDs(patch.MediaIDs.Value)
		flags.MediaIDs = true
	}
	if patch.MediaExcludeByAccount.Set {
		merged.MediaExcludeByAccount = NormalizeMediaExcludeByAccount(patch.MediaExcludeByAccount.Value, merged.MediaIDs)
		flags.MediaExcludeByAccount = true
	}
	if patch.Draft.Set {
		merged.Draft = patch.Draft.Value
		flags.Draft = true
	}

	if patch.AccountContentOverride.Set {
		merged.AccountContentOverride = NormalizeAccountContentOverride(patch.AccountContentOverride.Value, merged.TargetAccounts)
		flags.Versions = true
	} else {
		merged.AccountContentOverride = VersionsToOverrideMap(versions)
	}

	return merged, flags
}

func cloneMediaExclude(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}
