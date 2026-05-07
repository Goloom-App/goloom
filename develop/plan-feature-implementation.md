# Plan: Full Feature Implementation & Security Architecture

This document compares the desired features (from the feature catalog) with the current codebase and provides an implementation roadmap, including foundational security architecture decisions.

## 1. Feature Gap Analysis & Implementation Plan

### 1.1 Content Creation & Optimization
| Feature | Current State | Plan |
| :--- | :--- | :--- |
| **Multi-Platform Composer** | Single content for all. | Extract composer to component; implement tabs for per-platform overrides using `PostVersion` model. |
| **Real-time Preview** | Simple text preview. | Create platform-specific renderers (Mastodon/BlueSky) to simulate feed appearance. |
| **Media Library** | No central library. | Implement `Media` model/store; add workspace-scoped media explorer with local library persistence. |
| **Optimization Tools** | None. | Implement `HashtagGroup` and `Snippet` models; add insertion UI to composer. |

### 1.2 Scheduling & Automation
| Feature | Current State | Plan |
| :--- | :--- | :--- |
| **Queue & Time Slots** | Date-time picker only. | Implement `PostingSlot` model; add "Add to Queue" button that finds next available slot. |
| **Smart Automation** | None. | Implement "First Comment" logic in `scheduler.go` worker. |

### 1.3 Analytics & Insights
| Feature | Current State | Plan |
| :--- | :--- | :--- |
| **Unified Dashboard** | Basic admin counts. | Implement `PostMetric` storage; create workspace-wide aggregation API; add time-series charts to frontend. |
| **Post Tracking** | None. | Add UTM parameter auto-generation to URL fields; poll providers for daily engagement snapshots. |

### 1.4 Workspace & Team Management
| Feature | Current State | Plan |
| :--- | :--- | :--- |
| **Multi-Tenancy** | Strong isolation. | Maintain current architecture; ensure all new models are UUID-scoped to `team_id`. |
| **Account Health** | None. | Add `access_token_expires_at` to schema; add cron job to flag accounts needing re-auth. |

---

## 2. Security Architecture Decisions (Archived)

The following architectural patterns are foundational to the project's security and must be maintained during implementation:

### 2.1 Data Protection
*   **Token Encryption:** All external API tokens (Mastodon, BlueSky, etc.) MUST be encrypted at rest using **AES-256-GCM** (as implemented in `internal/security/security.go`). The master key is derived via SHA-256 from the environment variable.
*   **ID Strategy:** Use **UUID v4** for all primary keys to prevent ID enumeration attacks.

### 2.2 Authentication & Authorization
*   **Identity:** Support for **OpenID Connect (OIDC)** with PKCE for secure browser-based logins.
*   **Bearer Auth:** API access via OIDC ID Tokens or long-lived hashed API tokens (SHA-256).
*   **RBAC:** Middleware-enforced Role-Based Access Control (`RequireTeamRole`). New features must strictly map to `Owner`, `Editor`, `Viewer`, and the new `Contributor` role.

### 2.3 System Integrity
*   **Rate Limiting:** IP-based rate limiting on all API endpoints (standard: 60 req/min).
*   **CORS:** Strict origin validation based on `ALLOWED_ORIGINS` configuration.
*   **Input Handling:** Sanitization and validation of all content payloads to prevent XSS (especially in real-time previews).

---

## 3. Implementation Phases

1.  **Phase 1 (Infrastructure):** Database schema updates for Metrics, Versions, Slots, and Media. Implement encryption-aware store methods.
2.  **Phase 2 (Automation):** Implement the `FetchMetricsJob` and `AccountHealthJob` in the background scheduler.
3.  **Phase 3 (Core UI):** Refactor `App.tsx` into modular views; implement the Multi-platform Composer and Analytics Dashboard.
4.  **Phase 4 (Integrations):** Add external provider `GetMetrics` implementations.
