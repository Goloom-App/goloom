# Workflow: Account & Team Management

This document details the internal logic and workflows for managing multi-tenant environments and social media integrations.

## 1. Workspace-Centric Architecture
The application is built around the concept of "Workspaces." A workspace is an isolated environment that serves as a container for all resources.

*   **Isolation:** Data (posts, media, accounts, analytics) is strictly scoped to a Workspace ID.
*   **Ownership:** A workspace is owned by a primary user but can support multiple members.
*   **Context Switching:** The UI allows users to switch between workspaces, which updates the application state to point to the new Workspace ID.

## 2. Team & Role Management
Team members are managed within the context of a workspace using Role-Based Access Control (RBAC).

*   **User Invitation:** Owners/Admins invite users via email.
*   **Roles:**
    *   **Admin:** Full access to settings, billing, and user management.
    *   **Editor:** Can manage accounts, create/schedule posts, and view analytics.
    *   **Viewer:** Read-only access to the calendar and analytics.
*   **Workflow:**
    1.  Admin sends invitation.
    2.  User accepts and is linked to the `workspace_user` pivot table with a specific role.
    3.  Middleware validates the user's role before allowing access to specific API endpoints (e.g., `deleteAccount`, `updateSettings`).

## 3. Social Media Account Onboarding
The onboarding process uses a standardized OAuth 2.0 flow but is tailored for self-hosting.

*   **Provider Configuration:** The application owner must first input API Credentials (Client ID/Secret) for each platform (e.g., X, Facebook) in the global settings.
*   **Connection Flow:**
    1.  **Initiation:** User clicks "Add Account" within a workspace.
    2.  **Redirection:** Application redirects to the social platform's OAuth page with a `state` parameter containing the workspace context.
    3.  **Callback:** The platform redirects back to the `callback` URL with an authorization code.
    4.  **Persistence:** The `AccountsController` exchanges the code for a Bearer Token and Refresh Token.
    5.  **Metadata Extraction:** The system fetches the profile information (Name, Avatar, Handle, Platform ID) and saves it to the `accounts` table.
    6.  **Linking:** The account is linked to the current active workspace.
