# Frontend Refactoring & Modernization Plan (PWA 2026)

## Background & Motivation
The current frontend works well mechanically (e.g., the Sidebar and Preview mode are effective), but the UI lacks consistency and feels outdated. Components are often built in isolation rather than reusing a core design system, leading to visual inconsistencies. The light and dark themes have suboptimal color palettes, the layout wastes space, and specific views (like the Calendar) have layout bugs. 

The goal is to elevate the UI to a modern, space-efficient, **"Business-like" 2026 standard PWA**. This means shifting away from "playful" elements towards a clean, professional SaaS aesthetic (inspired by modern high-end dashboards), optimizing space, and improving maintainability through a modular component architecture.

## Scope & Impact
- **Modular Architecture:** Refactor the monolithic `App.tsx` into logical, modular components and hooks to improve maintainability and developer experience.
- **Design System & Components:** Audit and consolidate existing UI elements into a strict, reusable component library (e.g., buttons, inputs, cards, dialogs).
- **Professional Business Design:** Adopt a high-end SaaS aesthetic—cleaner lines, subtle borders, sophisticated typography, and a "Business-first" feel.
- **Theming & Brand Colors:** 
  - Redesign light and dark mode palettes for high readability and professional look.
  - Implement a **Team Color Picker** in settings to allow teams to define their own brand color, which will be used as the primary accent color.
- **Advanced Navigation:** 
  - Make the **Sidebar collapsible** in desktop view to maximize workspace.
  - Implement a **User Profile Menu**: Move Account settings, Administration, and Logout into a clean dropdown/menu triggered by clicking the user icon/name.
- **Layout & Space Efficiency:** Optimize margins, paddings, and typography scaling across all views.
- **Calendar View:** Fix the CSS grid implementation in the calendar (fixed/fractional columns).
- **PWA Enhancements:** Ensure the application behaves natively on mobile and desktop.

## Proposed Solution

### 1. Component Library & Modular Refactor
- **Break down `App.tsx`:** Identify core logic (routing, state management) and UI sections (Shell, Views, Modals) and move them into dedicated files.
- **UI Kit Consolidation:** Refine `frontend/src/components/ui/` with standardized components following the new "Business" design language.

### 2. Theme & Brand Customization
- **Sophisticated Palette:** Use a modern palette (e.g., deep slates/blacks for dark mode, crisp whites/grays for light mode).
- **Dynamic Brand Colors:** Introduce CSS variables for the primary brand color (e.g., `--brand-primary`). Add a color picker in team settings to update this value in the database and apply it globally for that team.

### 3. Navigation & Layout Refinement
- **Collapsible Sidebar:** Implement a toggle state for the sidebar. When collapsed, show only icons; when expanded, show icons and text. Store preference in local storage.
- **User Settings Menu:** Create a `UserMenu` component. Replace static links with a unified dropdown menu for a cleaner header/sidebar.
- **Space Optimization:** Tighten up the `AppShell` and content areas. Use a consistent spacing system (e.g., 4px/8px increments).

### 4. Calendar Layout Fix
- Update the CSS Grid definition for the Calendar view to use `minmax(0, 1fr)` for all columns, ensuring they remain equal and stable regardless of header content.

### 5. PWA Modernization
- Ensure the `manifest.json` and meta tags are up-to-date.
- Optimize touch targets and safe-area insets for the best mobile experience.

## Phased Implementation Plan

1. **Phase 1: Foundation, Theming & Refactoring**
   - Redefine CSS variables for the new "Business" look and brand color support.
   - Start breaking down `App.tsx` into logical sub-components (e.g., `MainLayout`, `RouteController`).
2. **Phase 2: Navigation & Component Refactor**
   - Implement the collapsible Sidebar and the User Profile Menu.
   - Extract and standardize core UI components.
3. **Phase 3: Team Features & Layout Fixes**
   - Add the Team Color Picker and dynamic accent color logic.
   - Fix the Calendar grid CSS.
   - Apply the "Compact" layout adjustments.
4. **Phase 4: Mobile & PWA Polish**
   - Test touch targets, safe-area insets, and overall mobile responsiveness.

## Verification
- Visual regression testing across Light, Dark, and Custom Brand themes.
- Verification of Sidebar collapse behavior across different screen sizes.
- Testing the modularized code for identical functionality to the original `App.tsx`.
- Calendar grid stability testing.
