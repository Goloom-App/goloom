# Goloom Project Roadmap

This roadmap outlines the logical progression for implementing the full feature set of Goloom. It is designed to be followed by AI agents or developers to ensure a stable and secure evolution of the codebase.

## Overview
The goal is to transform Goloom from a basic social media scheduler into a robust, multi-tenant, media-rich social media management platform with deep analytics and team collaboration features.

---

## Phase 1: Foundation & Security Infrastructure
*Goal: Ensure the database and core models support multi-tenancy, encryption, and secure account management.*

- [x] **Account & Team Model Migration**
    - Implement personal teams and multi-tenant isolation.
    - [Reference: docs/plan-account-model.md](docs/plan-account-model.md)
    - [Reference: docs/workflow-management.md](docs/workflow-management.md)
- [x] **Security Hardening**
    - Fix rate limiter memory leak and SSRF vulnerabilities.
    - Enforce mandatory encryption keys in production.
    - [Reference: docs/security-issues.md](docs/security-issues.md)
    - [Reference: docs/plan-feature-implementation.md (Section 2)](docs/plan-feature-implementation.md)
- [x] **Schema Updates**
    - Add `access_token_expires_at` and UUID-based scoping for all new models.
    - [Reference: docs/plan-social-integration-technical.md](docs/plan-social-integration-technical.md)

## Phase 2: Enhanced Social Integrations & Publishing
*Goal: Support media uploads, platform-specific overrides, and robust publishing workflows.*

- [x] **Provider Interface Refactor**
    - Update `SocialMediaProvider` to support media uploads and expanded `PublishRequest`/`PublishResult`.
    - [Reference: docs/plan-social-integration-technical.md](docs/plan-social-integration-technical.md)
    - [Reference: docs/social-integration-technical.md](docs/social-integration-technical.md)
- [x] **Mastodon Media Support**
    - Implement two-step publishing (Upload -> Attach) for Mastodon.
- [x] **Multi-Platform Composer Backend**
    - Support `PostVersion` model to allow different content per platform.
    - [Reference: docs/plan-feature-implementation.md (Section 1.1)](docs/plan-feature-implementation.md)

## Phase 3: Automation & Background Services
*Goal: Implement reliable background jobs for account health and metric synchronization.*

- [x] **Account Health Monitoring**
    - Implement `AccountHealthJob` to flag expired tokens.
    - [Reference: docs/plan-feature-implementation.md (Section 1.4)](docs/plan-feature-implementation.md)
- [x] **Metric Synchronization**
    - Implement `FetchMetricsJob` with daily snapshot logic.
    - [Reference: docs/analytics-parameters.md](docs/analytics-parameters.md)
    - [Reference: docs/plan-analytics-implementation.md](docs/plan-analytics-implementation.md)

## Phase 4: Analytics Infrastructure & API
*Goal: Build the data processing layer for performance insights.*

- [x] **Analytics Snapshot Storage**
    - Implement `post_metrics_history` and delta calculation logic.
    - [Reference: docs/plan-analytics-implementation.md](docs/plan-analytics-implementation.md)
- [x] **Aggregation API**
    - Create endpoints for summary, charts, and post-ranking.
    - [Reference: docs/plan-analytics-implementation.md (Section 2)](docs/plan-analytics-implementation.md)

## Phase 5: Frontend Modularization & UI Implementation
*Goal: Deliver a modern, interactive UI for all new features.*

- [x] **Modular UI Refactor**
    - Split `App.tsx` into views and layout components.
    - [Reference: docs/ui-views-catalog.md](docs/ui-views-catalog.md)
- [x] **Advanced Composer**
    - Build the multi-platform tabs, real-time preview, and media gallery.
    - [Reference: docs/ui-views-catalog.md (Section 3)](docs/ui-views-catalog.md)
- [x] **Analytics Dashboard**
    - Implement charts (using Recharts) and summary tiles.
    - [Reference: docs/plan-analytics-implementation.md (Section 3)](docs/plan-analytics-implementation.md)
- [x] **Media Library View**
    - Implement the workspace-scoped media explorer with Unsplash/Giphy integration.
    - [Reference: docs/ui-views-catalog.md (Section 4)](docs/ui-views-catalog.md)

---

## How to use this Roadmap
Each phase should be tackled sequentially. Agents should:
1.  Read the linked planning documents for the current task.
2.  Perform a "Gap Analysis" against the current codebase.
3.  Implement the changes (DB -> Internal Logic -> API -> Frontend).
4.  Validate against the security mandates in `docs/plan-feature-implementation.md`.
