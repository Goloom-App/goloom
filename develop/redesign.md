# UI/UX Redesign Implementation Plan (2026)

## Overview
This document outlines the strategic plan for overhauling the `goloom` frontend to achieve a world-class PWA experience while maintaining a professional, multi-column desktop layout. The redesign focuses on "Native-Feel" interactions, ultra-fast performance, and robust light/dark theming without external dependencies.

## 1. Foundational Stack (2026 Standards)
*   **Core:** React 19 (utilizing latest Hooks and performance optimizations).
*   **Interactions:** **Radix UI Primitives**. We use unstyled, accessible components (Dialog, Dropdown, Tabs) to ensure PWA-standard behaviors (focus management, escape keys, touch targets) while maintaining full control over CSS.
*   **Styling:** **CSS Design Tokens (Vanilla CSS)**.
    *   Avoid utility-first bloat (Tailwind) to keep the self-hosted binary small.
    *   Use CSS Variables for all semantic colors (`--surface`, `--accent`, `--text-soft`).
    *   Implement light/dark mode via `[data-theme]` on the `.app-shell` container.
*   **Icons:** **Lucide React**. Locally bundled, tree-shaken SVG icons for a consistent 2026 look.
*   **Typography:** System font stacks with a preference for "Geist" or "Inter" to maintain a clean, tool-like aesthetic.

## 2. PWA-First Architecture
The PWA experience has been promoted from an afterthought to the primary design driver.

### A. Bottom Navigation (Mobile)
*   **Context:** Mobile users expect primary actions within thumb-reach.
*   **Implementation:** A fixed bottom bar with:
    *   Home (Dashboard)
    *   Calendar (Schedule)
    *   **Center FAB:** "New Post" (Primary Action)
    *   Media (Library)
    *   More (Drawer Trigger)

### B. Side Drawer (Management Hub)
*   **Context:** Secondary settings (Teams, Accounts, Admin) clutter mobile views.
*   **Implementation:** A Radix-based Drawer that slides from the bottom/side, containing:
    *   Workspace/Team Switcher.
    *   Management links (Accounts, Teams, Analytics).
    *   User Profile & Logout.
    *   Theme Toggle.

### C. Desktop Transformation
*   **Layout:** Seamlessly shifts to a triple-column layout on screens > 1024px.
*   **Sidebar:** Replaces the Bottom Nav with a persistent left sidebar.
*   **Preview Column:** A dedicated right-hand column for "Live Previews," ensuring what you see is what you get without navigating away.

## 3. Visual Language & Aesthetics
*   **Mesh Gradients:** Subtle, blurred background shapes (`radial-gradient`) that provide depth without distracting from content.
*   **Glassmorphism:** Use of `backdrop-filter: blur()` and semi-transparent surfaces for a modern, layered feel.
*   **Micro-animations:** 
    *   `view-fade-in`: Smooth 300ms transition when switching sections.
    *   Scale transforms on button clicks for tactile feedback.
*   **Safe Areas:** Strict adherence to `env(safe-area-inset-*)` to support notched displays and home indicators on iOS/Android.

## 4. Light & Dark Theming Strategy
The system uses a "Single Source of Truth" approach:
1.  **Definitions:** All colors are defined twice in `index.css` (once for `.app-shell[data-theme="dark"]` and once for `light`).
2.  **Inheritance:** Components only reference variables (e.g., `background: var(--surface)`).
3.  **Persistence:** The choice is saved in `localStorage` and optionally synced with the `prefers-color-scheme` system setting.

## 5. Deployment & Self-Hosting
*   **Zero CDNs:** Every icon, font, and script is bundled into the `/dist` folder.
*   **Binary Embedding:** The Go backend embeds the `/dist` folder, ensuring the application works in air-gapped or local environments without internet access.

## 6. Implementation Roadmap

### Phase 1: Shell & Foundation (Completed)
- [x] Install Radix UI & Lucide.
- [x] Establish Design Token system in `index.css`.
- [x] Implement `AppShell` with responsive logic.
- [x] Build `BottomNav` and `Sidebar`.

### Phase 2: View Refinement (In Progress)
- [x] Redesign Dashboard with Sparkline charts.
- [x] Update Post Card layouts for better information density.
- [x] Port Post Composer to a full-screen mobile modal.
- [ ] Implement swipe-to-action gestures for the Timeline view.

### Phase 3: PWA Optimization
- [ ] Audit Manifest.json for proper "Standalone" display.
- [ ] Add Service Worker for basic asset caching.
- [ ] Implement Native Share API integration for media.

## 7. Quality Standards
*   **Accessibility:** 100% Radix-based focus management.
*   **Performance:** No layout shifts (CLS) during section switching.
*   **Touch:** Minimum touch target size of 44x44px for all interactive elements.
