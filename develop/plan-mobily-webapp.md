# Plan: Mobile-First Web Application (PWA) Evolution

This document outlines the strategy for transforming the Goloom frontend into a responsive, mobile-optimized web application that provides a native app-like experience while maintaining full desktop functionality.

## 1. Objectives
*   **Mobile-First UX:** Primary navigation and core workflows (scheduling, analytics) optimized for one-handed thumb use on small screens.
*   **App-like Feel:** PWA capabilities, smooth transitions, and touch-optimized components.
*   **Adaptive Layout:** Seamlessly scale from mobile (<600px) to tablet (600px-1024px) and desktop (>1024px).
*   **Cleanup:** Formally remove deprecated Giphy and Unsplash integrations.
*   **Maintainability:** Consolidate management views to reduce cognitive load and navigation depth.

## 2. Navigation & Layout Refactor

### 2.1 Adaptive Shell
*   **Desktop:** Retain the persistent left sidebar for rapid switching.
*   **Tablet:** Use a slim sidebar (icons only) or a hidden drawer to maximize content area.
*   **Mobile:** Replace the sidebar with a **Bottom Navigation Bar** (5 items max).

### 2.2 Bottom Navigation (Mobile)
The current mobile navigation is overcrowded. It will be refined to:
1.  **Schedule:** (Calendar icon) Main dashboard and scheduling list.
2.  **Analytics:** (Chart icon) Performance insights.
3.  **Compose:** (Central floating/primary button) Primary action.
4.  **Media:** (Image icon) Media library.
5.  **More:** (Menu icon) Access to Teams, Accounts, Settings, and Admin.

### 2.3 Management Consolidation
"Accounts", "Teams", "Settings", and "Admin" will be merged into a unified **Management View**:
*   On Desktop: A sidebar or top-tab sub-navigation within the Settings section.
*   On Mobile: A list-based menu under the "More" tab, leading to full-screen sub-views.

## 3. View-Specific Adaptations

### 3.1 Schedule & Calendar
*   **Mobile:** Default to a vertical **List View** (Timeline) instead of the month grid.
*   **Interaction:** Swipe left/right to change days/weeks.
*   **Detail:** Tapping a post opens a full-screen detail/preview view.

### 3.2 Post Composer
*   **Full-Screen:** The composer will occupy 100% of the viewport on mobile.
*   **Tabs:** Use a top-tab bar for switching between platform-specific overrides.
*   **Preview:** Social preview will be a separate tab or a toggleable overlay rather than a side-by-side column.

### 3.3 Analytics
*   **Responsive Charts:** Recharts will be configured to use `ResponsiveContainer`.
*   **Summary Cards:** Stack vertically on mobile; use a grid on desktop.
*   **Touch:** Optimize tooltips for long-press/tap interaction.

## 4. App-Like Enhancements (PWA)

### 4.1 PWA Infrastructure
*   **Manifest:** Add `manifest.json` with icons, theme colors (`#1d4ed8`), and `display: standalone`.
*   **Icons:** Generate high-quality PNG icons from the SVG logo for home screen installation.
*   **Safe Areas:** Ensure layout respects `env(safe-area-inset-*)` for modern notch/home-bar devices.

### 4.2 Interaction Design
*   **Touch Targets:** Ensure all buttons and links have at least 44x44px hit areas.
*   **Pull-to-Refresh:** Implement for the Schedule and Analytics views to refresh data.
*   **Transitions:** Add subtle slide/fade transitions between major sections to mimic native navigation.

## 5. Media Library Cleanup
*   Remove all references and UI stubs for Giphy and Unsplash.
*   Focus purely on local/team-uploaded media as the primary source.
*   Update documentation (`ROADMAP.md`, `feature-catalog-content.md`, etc.) to reflect this change.

## 6. Implementation Roadmap

### Phase A: Foundation & PWA
1.  Create `manifest.json` and add PWA meta tags to `index.html`.
2.  Implement `useIsMobile` and `useIsTablet` hooks for component-level adaptation.
3.  Standardize the "Bottom Navigation" component.

### Phase B: Layout Refactor
1.  Update `App.tsx` to switch between Sidebar (Desktop) and Bottom Nav (Mobile).
2.  Implement the unified "Management Hub" view.
3.  Refactor the `App-Shell` CSS to handle safe areas and adaptive columns.

### Phase C: View Optimization
1.  Optimize `ContentCalendarView` for mobile (List View toggle).
2.  Make `PostComposer` full-screen on mobile with tabbed preview.
3.  Ensure `AnalyticsView` charts are responsive and touch-friendly.

### Phase D: Cleanup & Polish
1.  Purge Giphy/Unsplash code and documentation.
2.  Audit touch targets and spacing across all views.
3.  Final testing on iOS/Android browsers.
