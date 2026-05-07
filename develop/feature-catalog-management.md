# Feature Catalog: Workspace & Team Management

Tools for managing multi-user environments and multiple client profiles.

## 1. Multi-Tenant Workspaces
*   **Logical Isolation:** Each workspace has its own connected accounts, media library, schedules, and analytics, ensuring no data leakage between clients or brands.
*   **Switching:** Seamless interface for users to jump between different workspaces they have access to.

## 2. Team Collaboration & Roles
*   **User Management:** Invite team members to specific workspaces with different access levels.
*   **Role-Based Access Control (RBAC):**
    *   **Owner/Admin:** Full control over settings, integrations, and users.
    *   **Editor:** Can create, edit, and schedule posts.
    *   **Viewer:** Read-only access to analytics and the calendar.
    *   **Contributor:** Can create drafts but cannot schedule or publish (requires approval).

## 3. Platform Integrations
*   **OAuth Management:** Securely connect and manage API tokens for various platforms (Mastodon, BlueSky, X, Facebook, LinkedIn, etc.).
*   **Account Health Monitoring:** Visual indicators if a platform token has expired or if an account needs re-authentication.
