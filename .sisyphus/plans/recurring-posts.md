# Recurring Posts Feature — Implementation Plan

## Units (Prioritized)

| # | Unit | Dependencies | Size | Description |
|---|------|-------------|------|-------------|
| A | Recurrence UI form | none | large | Replace JSON textarea with visual recurrence builder |
| B | Ordinal weekday recurrence | (A) new kind enum | medium | "erster Freitag im Monat" support |
| C | Template variable expansion | none | medium | Richer variables, preview in UI |
| D | Skip + Shift occurrences | (A) occurrence list UI | medium | Extend skip to allow shift by N days |
| E | Announcement posts | (C) variables, (D) shift | large | Paired template for pre-event posts |

## Unit A: Recurrence UI Form

### Acceptance Criteria
- Kind selector: weekly / monthly_dom / monthly_anchor_offset
- Weekly: 7 weekday toggles (Mon-Sun), hour input, minute input
- Monthly DOM: day-of-month (1-31), hour, minute (clamp to max days)
- Monthly Anchor Offset: anchor day (1-31), offset days (-30..30), hour, minute
- Timezone dropdown with search (common ~40 tz)
- Preview pane showing next 5 occurrences in human-readable format
- Output: valid `recurrence_json` string matching `RecurrenceRule` struct
- Cancel button discards changes
- Edit mode for existing templates (reuses same form, pre-filled)
- i18n keys for all new form labels

### Files to modify
- `frontend/src/views/recurring/RecurringPostsView.tsx` — major restructure
- `frontend/src/index.css` — new form styles
- `locales/de.json` + `locales/en.json` — new i18n keys
- `frontend/src/views/recurring/` — new components (RecurrenceForm, OccurrencePreview)
- `frontend/src/api.ts` — no changes (API stays same, JSON output)

### Edge cases
- Day-of-month > max days in month → clamp to last day
- Negative offset + anchor = day 1 → clamp at 1
- Empty weekdays → disable Save
- Invalid timezone → show error
- Date overflow (e.g., Feb 31) → show warning

## Unit B: Ordinal Weekday Recurrence

### Acceptance Criteria
- New kind: `monthly_ordinal_weekday`
- Fields: ordinal (1-5, -1=last), weekday (0=Sun..6=Sat), hour, minute, timezone
- `NextOccurrence()` correctly computes first/last given weekday of month
- Validation: ordinal 1-5 or -1, weekday 0-6
- UI: ordinal select (1st..5th, last) + weekday select + time
- Preview shows correct dates

### Files to modify
- `internal/scheduling/recurrence.go` — add kind constant, NextOccurrence branch, validation
- `internal/scheduling/recurrence_test.go` — table-driven tests
- `frontend/src/views/recurring/RecurringPostsView.tsx` — add form section
- `locales/de.json` + `locales/en.json` — new keys

### Edge cases
- Month with <5 of given weekday (e.g., 5th Friday in 30-day month) → last occurrence
- -1 (last) weekday shifts with month length
- Timezone boundary: occurrence date may differ in UTC

## Unit C: Template Variable Expansion

### Acceptance Criteria
- New variables: `{day+N}`, `{month+N}`, `{weekday}`, `{weekday_name}`
- `ExpandDynamicVariables()` updated for all new vars
- Frontend preview: show expanded content with real NextOccurrence date
- Counter shown with actual next value
- Existing `{year}`, `{month}`, `{day}`, `{counter}` unchanged

### Files to modify
- `internal/domain/templatevars.go` — add new variable expansions
- `internal/domain/templatevars_test.go` — tests for new vars
- `frontend/src/views/recurring/RecurringPostsView.tsx` — preview panel
- `internal/scheduler/scheduler.go` — no changes (expansion happens at publish)
- `locales/de.json` + `locales/en.json` — hint text for new vars

### Security
- Variable expansion must escape/sanitize output (no injection into stored content)
- Preview is client-side only; server still does final expansion at publish

## Unit D: Skip + Shift

### Acceptance Criteria
- Existing skip (postpone indefinitely) preserved
- New: shift by N days (reschedule to occurrence + N days)
- API: `POST .../skip` accepts optional `"shift_days": N`
- UI: occurrence list shows next 3 dates; each has "Skip" and "Shift +N/-N" buttons
- Shifted occurrence stored in `post_template_skips` with `shift_to` timestamp
- Counter behavior: skip = counter still increments; shift = counter same

### Files to modify
- `api/post_templates.go` — extend skip body with shift_days
- `api/http.go` — route unchanged
- `internal/store/store.go` — add ShiftPostTemplateOccurrence to interface
- `internal/store/postgres/post_templates.go` — implement shift
- `internal/store/sqlite/post_templates.go` — implement shift
- `internal/domain/models.go` — optional ShiftTo field on skip
- `internal/scheduler/scheduler.go` — handle shift in materializePostTemplates
- Schema (both): add `shift_to timestamptz` to `post_template_skips`
- `frontend/src/views/recurring/RecurringPostsView.tsx` — shift UI
- `frontend/src/api.ts` — update skip method signature
- `locales/de.json` + `locales/en.json`

### Edge cases
- Shift_to = occurrence_at → no-op
- Shift_to in past → materialize immediately? (edge: treat as due)
- Multiple shifts on same occurrence → last shift wins
- Shift + skip interaction → shift overrides skip

## Unit E: Announcement Posts

### Acceptance Criteria
- Template can reference another template as "announcement for"
- Announcement template materializes N days before main template occurrence
- Variables in announcement: `{main_day}`, `{main_month}`, `{main_weekday_name}` + `{counter}`
- UI: "Create announcement" checkbox in template editor, selects main template
- Preview shows both announcement date and main event date

### Open Design Questions
1. Data model: `announces_template_id` nullable FK on `post_templates`? Or paired-group concept?
2. Schedule: announcement materializes when its own recurrence fires, but uses main template's next occurrence date for variable expansion?
3. UI: announcement editor is subset of template editor (no recurrence of its own — uses parent's)?
4. For "first Friday of month, announce 2 days before" — announcement template needs NO recurrence of its own; dates derive from parent.

### Files to modify (tentative)
- `internal/domain/models.go` — add announces_template_id
- Schema (both) — add announces_template_id column
- `internal/scheduler/scheduler.go` — handle announcement materialization
- `frontend/src/views/recurring/RecurringPostsView.tsx` — announcement UI
- `locales/` — new keys

## Execution Order

```
Unit A ──→ Unit D ──→ Unit E
   │                    ↑
   ↓                    │
Unit B ──→ Unit C ──────┘
```

- A + C parallel (different files, no overlap)
- A before D (need occurrence list UI for shift buttons)
- C before E (need variable expansion for announcement)
- B parallel to C (different layers)
- E last (depends on C for variables, D for shift mechanism)

## Testing Strategy

| Unit | Backend Tests | Frontend Tests |
|------|--------------|----------------|
| A | — (Pure frontend) | Playwright: fill form, verify JSON output, verify preview |
| B | Go: NextOccurrence ordinal weekday table tests | Playwright: select ordinal+weekday, verify preview dates |
| C | Go: ExpandDynamicVariables new vars | Playwright: type content with {day+2}, verify preview expansion |
| D | Go: shift logic in materialize | Playwright: click shift, verify rescheduled |
| E | Go: announcement materialization | Playwright: create announcement template, verify both posts |

## Security Constraints

- All API endpoints require `RoleEditor+` (existing auth middleware)
- Recurrence JSON validated server-side (ParseRecurrenceJSON)
- Template content goes through same sanitization as regular posts
- Variable expansion must not introduce injection vectors
- Shift_days limited to -365..+365 (validation)
