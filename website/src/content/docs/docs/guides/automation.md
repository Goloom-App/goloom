---
title: Automation
description: Recurring post templates and RSS feed rules for hands-off scheduling.
---

**Automation** combines time-based recurring posts and RSS feed imports in one place. Optional **AI enhancement** rewrites drafts using the same voice engine as [AI features](/docs/guides/ai-features/).

Open **Automation** from the sidebar (editor or owner access required).

## Time-based recurring posts

Recurring **templates** expand into scheduled posts on a cadence you define.

### Template fields

- **Title template** — supports variables such as `{year}`, `{month}`, `{day}`, `{counter}`, `{weekday_name}`.
- **Content** — the post body; variables expand when each occurrence is created.
- **Target accounts** — one or more connected accounts.
- **Recurrence** — weekly, monthly (day of month), monthly with anchor offset, or ordinal weekday (for example “2nd and 4th Friday”).
- **Timezone**, hour, and minute for each occurrence.

### Managing templates

From the template list you can **pause**, **resume**, **skip next**, **shift** the next occurrence, or **delete** a template.

**Pre-schedule posts** (materialize horizon) creates drafts days or weeks ahead so they appear on the [Content calendar](/docs/guides/content-calendar/) for review before publish time.

### Announcements

Templates can post a separate **announcement** a fixed number of days before the main occurrence, optionally to different accounts.

### AI enhancement

When AI is enabled for the workspace, recurring templates can request AI-generated text for the main post, the announcement, or both. Generated drafts may land in the [Review queue](/docs/guides/review-queue/) depending on team settings.

## RSS feeds

RSS rules watch a feed URL and create posts when new articles appear.

Configure:

- **Feed URL** and target accounts
- **Title/content template** for each new item
- **Enhance with AI** — optional instructions sent with the article text to the voice engine
- **Tonality override** — temporary style override for feed posts

Imported drafts typically enter the **review queue** so an editor can approve, edit, or discard them before publishing.

Administrators can trigger a background RSS sync from the admin panel when testing feeds.

## Permissions

- **Editors** and **owners** create and edit automation rules.
- **Viewers** can see outcomes (calendar, review queue) but not change rules.

## Related guides

- [Review queue](/docs/guides/review-queue/) — approve RSS and AI drafts
- [AI features](/docs/guides/ai-features/) — voice profile and generation
- [Content calendar](/docs/guides/content-calendar/) — see pre-scheduled occurrences
