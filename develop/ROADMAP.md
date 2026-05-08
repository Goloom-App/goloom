# Goloom Project Roadmap

This roadmap outlines the logical progression for implementing the full feature set of Goloom. It is designed to be followed by AI agents or developers to ensure a stable and secure evolution of the codebase.

---

## 📖 Core Documentation
- **Detailed Feature Catalog & Specifications:** [REQUIREMENTS.md](REQUIREMENTS.md)
- **Advanced Scheduling & Recurrence Engine:** [ADVANCED_SCHEDULING.md](ADVANCED_SCHEDULING.md)

---

## Phase 1: Foundation & Security Infrastructure
*Goal: Ensure the database and core models support multi-tenancy, encryption, and secure account management.*

- [x] **Account & Team Model Migration**
    - Implement personal teams and multi-tenant isolation.
    - [Ref: REQUIREMENTS.md (Section 2)](REQUIREMENTS.md#2-workspace--multi-tenancy)
- [x] **Security Hardening**
    - Fix rate limiter memory leak and SSRF vulnerabilities.
    - Enforce mandatory encryption keys in production.
    - [Ref: REQUIREMENTS.md (Section 9)](REQUIREMENTS.md#9-security--integrity)
- [x] **Schema Updates**
    - Add `access_token_expires_at` and UUID-based scoping for all new models.

## Phase 2: Enhanced Social Integrations & Publishing
*Goal: Support media uploads, platform-specific overrides, and robust publishing workflows.*

- [x] **Provider Interface Refactor**
    - Update `SocialMediaProvider` to support media uploads and expanded `PublishRequest`/`PublishResult`.
    - [Ref: REQUIREMENTS.md (Section 4 & 5)](REQUIREMENTS.md#4-social-media-integrations)
- [x] **Mastodon Media Support**
    - Implement two-step publishing (Upload -> Attach) for Mastodon.
- [x] **Multi-Platform Composer Backend**
    - Support `PostVersion` model to allow different content per platform.
    - [Ref: REQUIREMENTS.md (Section 5)](REQUIREMENTS.md#5-content-creation--optimization)

## Phase 3: Automation & Background Services
*Goal: Implement reliable background jobs for account health and metric synchronization.*

- [x] **Account Health Monitoring**
    - Implement `AccountHealthJob` to flag expired tokens.
    - [Ref: REQUIREMENTS.md (Section 4)](REQUIREMENTS.md#4-social-media-integrations)
- [x] **Metric Synchronization**
    - Implement `FetchMetricsJob` with daily snapshot logic.
    - [Ref: REQUIREMENTS.md (Section 8)](REQUIREMENTS.md#8-analytics--insights)

## Phase 4: Analytics Infrastructure & API
*Goal: Build the data processing layer for performance insights.*

- [x] **Analytics Snapshot Storage**
    - Implement `post_metrics_history` and delta calculation logic.
    - [Ref: REQUIREMENTS.md (Section 8)](REQUIREMENTS.md#8-analytics--insights)
- [x] **Aggregation API**
    - Create endpoints for summary, charts, and post-ranking.

## Phase 5: Frontend Modularization & UI Implementation
*Goal: Deliver a modern, interactive UI for all new features.*

- [x] **Modular UI Refactor**
    - Split `App.tsx` into views and layout components.
- [x] **Advanced Composer**
    - Build the multi-platform tabs, real-time preview, and media gallery.
    - [Ref: REQUIREMENTS.md (Section 5)](REQUIREMENTS.md#5-content-creation--optimization)
- [x] **Analytics Dashboard**
    - Implement charts (using Recharts) and summary tiles.
    - [Ref: REQUIREMENTS.md (Section 8)](REQUIREMENTS.md#8-analytics--insights)
- [x] **Media Library View**
    - Implement the workspace-scoped media explorer with local library persistence.
    - [Ref: REQUIREMENTS.md (Section 6)](REQUIREMENTS.md#6-media-management)

## Phase 6: Mobile Optimization & PWA (Current Focus)
*Goal: Transform the platform into a responsive, app-like experience.*

- [x] **Adaptive Navigation**
    - Implement bottom navigation for mobile and refined sidebar for desktop.
    - [Ref: PWA_MOBILE_PLAN.md](PWA_MOBILE_PLAN.md)
- [x] **PWA Manifest & Assets**
    - Add `manifest.json`, theme colors, and standalone display support.
- [x] **Mobile-Specific Views**
    - Vertical list view for Calendar, full-screen composer.
- [x] **Cleanup**
    - Remove deprecated Giphy and Unsplash integrations.

## Phase 7: Advanced Scheduling & Dynamic Templates
*Goal: Implement logic-based recurrence, smart scheduling defaults, and a factory-based template engine.*

- [ ] **Dynamic Variable Engine**
    - Implement server-side replacement of `{year}`, `{month}`, `{day}`, and `{counter}` at publish time.
    - [Ref: REQUIREMENTS.md (Section 5)](REQUIREMENTS.md#5-content-creation--optimization)
- [ ] **Post Template & Recurrence Engine**
    - Implement `PostTemplate` model and the logic-based resolver (e.g., "3 days before X").
    - Build the background worker that generates individual `ScheduledPost` instances.
    - [Ref: ADVANCED_SCHEDULING.md](ADVANCED_SCHEDULING.md)
- [ ] **Smart Scheduling & Advanced UI**
    - Support team-level "Preferred Posting Windows" and "Default Timeslots".
    - Build a custom clock selector with 12h/24h toggle and engagement heatmap visualization.
- [ ] **Recurring Content Management View**
    - Create a dedicated "Recurring Posts" view in the Workspace section for managing templates and skipping iterations.

---

## How to use this Roadmap
Each phase should be tackled sequentially. Agents should:
1.  Read the relevant sections in [REQUIREMENTS.md](REQUIREMENTS.md).
2.  Perform a "Gap Analysis" against the current codebase.
3.  Implement the changes (DB -> Internal Logic -> API -> Frontend).
4.  Validate against security mandates.
