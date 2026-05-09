# UI/UX Redesign Implementation Plan (2026)

> **Status:** Planning
> **Target:** PWA-first progressive redesign with shared desktop experience
> **Current state assessment follows — all recommendations are grounded in observed codebase reality

---

## Table of Contents

1. [Current State Assessment](#1-current-state-assessment)
2. [Guiding Principles](#2-guiding-principles)
3. [Recommended Technology Stack (2026)](#3-recommended-technology-stack-2026)
4. [PWA-First Architecture](#4-pwa-first-architecture)
5. [Visual Language Overhaul](#5-visual-language-overhaul)
6. [Design Token System Expansion](#6-design-token-system-expansion)
7. [Component Architecture](#7-component-architecture)
8. [Mobile Navigation & Gesture System](#8-mobile-navigation--gesture-system)
9. [Desktop Layout Refinements](#9-desktop-layout-refinements)
10. [Animation & Motion System](#10-animation--motion-system)
11. [Data Layer & Offline Strategy](#11-data-layer--offline-strategy)
12. [Light & Dark Theming](#12-light--dark-theming)
13. [View-by-View Redesign Specification](#13-view-by-view-redesign-specification)
14. [Implementation Phases](#14-implementation-phases)
15. [Quality Standards & Verification](#15-quality-standards--verification)

---

## 1. Current State Assessment

### 1.1 What Exists Today

- **Stack:** React 19 + Vite 8 + TypeScript 6 + vanilla CSS
- **UI Primitives:** Radix UI (`@radix-ui/react-dialog`, `@radix-ui/react-dropdown-menu`, `@radix-ui/react-slot`, `@radix-ui/react-tabs`)
- **Icons:** Custom inline SVG component (`icons.tsx`) + Lucide React for nav icons — **two icon systems that should merge**
- **Charts:** Recharts v3.8.1
- **Dates:** date-fns v4
- **PWA:** Basic `manifest.json` with SVG icons — **no service worker, no offline support**
- **Auth:** Token-based bearer + OIDC
- **State:** React hooks only (useState/useEffect) — **no data fetching library**

### 1.2 What Works Well (Preserve)

| Feature | Why Keep |
|---|---|
| CSS Design Token system | Semantic variables (`--surface`, `--accent`, `--text`) — clean, maintainable, zero runtime |
| `[data-theme]` scoping | Clean dark/light separation without class toggling |
| Radix UI primitives | Excellent accessibility, already installed, industry standard |
| Single-column mobile → multi-column desktop | Responsive approach is sound |
| Glass-panel pattern | `backdrop-filter: blur()` with subtle borders — modern feel |
| Mesh gradient backgrounds | Subtle depth without distraction |
| View fade-in animation | Clean 300ms entrance |
| Custom SVG icon component | Tree-shakeable, no extra dependency |

### 1.3 What Needs Overhaul (Issues Found)

| Issue | Severity | Details |
|---|---|---|
| **No service worker** | 🔴 Critical | App cannot work offline. PWA install prompt won't fire consistently. |
| **Mixed icon systems** | 🟡 Medium | `icons.tsx` duplicates Lucide — some views use Lucide, some use custom SVG. Inconsistent. |
| **Inline styles everywhere** | 🟡 Medium | Sidebar, AppShell, NavItems, views use `style={{}}` heavily — breaks theming, hard to maintain |
| **No loading skeletons** | 🟡 Medium | Only text "Loading…" placeholders — poor perceived performance |
| **No toast/notification system** | 🟡 Medium | Status messages use inline banners — intrusive, not dismissible properly |
| **No animation system** | 🟢 Low | Only view-fade-in exists. No micro-interactions, no shared element transitions |
| **Mobile nav is basic** | 🟡 Medium | Bottom nav has 5 items + drawer. No swipe gestures, no pull-to-refresh |
| **No search/command palette** | 🟢 Low | Teams with many accounts/posts lack quick navigation |
| **Theme colors could be richer** | 🟢 Low | Current palette is functional but not distinctive |
| **No skeleton/empty states** | 🟡 Medium | Only `<p className="hint">` for empty states — no illustration or call-to-action |
| **Auth screen is minimal** | 🟢 Low | Functional but lacks visual personality for a first impression |
| **No proper error boundaries** | 🟡 Medium | Errors bubble to inline banner — no fallback UI per section |

---

## 2. Guiding Principles

1. **PWA-first, desktop-always**: Mobile experience drives design decisions. Desktop gets the same features in a multi-column layout.
2. **Zero CDN dependencies**: Every asset bundled into the Go binary for air-gapped self-hosting.
3. **Own your code**: Copy-paste component pattern (shadcn philosophy) — no black-box dependencies.
4. **Accessibility as foundation**: Radix handles ARIA; we handle semantics, contrast, and touch targets.
5. **Performance budget**: No layout shifts (CLS), minimal JS bundle, 60fps animations.
6. **Progressive enhancement**: PWA features (SW, offline, install) degrade gracefully.
7. **Design tokens everywhere**: No inline `style` props that bypass the theme system.

---

## 3. Recommended Technology Stack (2026)

### 3.1 Core (Keep)

| Library | Version | Reason |
|---|---|---|
| React | 19.x | Latest hooks, concurrent features |
| TypeScript | 6.x | Already on latest |
| Vite | 8.x | Already on latest |
| Radix UI | latest | Already installed, excellent a11y. Migrate to unified `radix-ui` package (Feb 2026 shadcn pattern) |
| date-fns | 4.x | Already installed |
| Lucide React | latest | Already installed — **use exclusively**, remove custom `icons.tsx` |

### 3.2 New Additions

| Library | Bundle (gzip) | Purpose | Priority |
|---|---|---|---|
| `vite-plugin-pwa` | ~3kb (devDep) | Service worker generation, manifest management, offline caching | 🔴 P0 |
| `@tanstack/react-query` v5 | ~12kb | Data fetching, caching, offline persistence, mutation queue | 🔴 P0 |
| `@tanstack/react-query-persist-client` | ~2kb | Persist query cache to IndexedDB for offline | 🔴 P0 |
| `idb-keyval` | ~1kb | IndexedDB wrapper for query persister | 🔴 P0 |
| `sonner` | ~2.5kb | Toast notifications — shadcn default, zero deps, accessible | 🟡 P1 |
| ~~`motion` (framer-motion)~~ | — | **Not recommended** — CSS-only approach preferred for bundle discipline | ❌ |
| `@formkit/auto-animate` | ~3kb | Zero-effort enter/exit animations for lists | 🟢 P2 |

### 3.3 Adopt shadcn/ui Component Patterns (Not the Dependency)

Since goloom is self-hosted in a Go binary, adding Tailwind + shadcn CLI would add build complexity and CSS bloat. **Instead, adopt the architectural patterns:**

1. **Component ownership**: Copy-paste Radix-based components directly into `frontend/src/components/ui/`
2. **CSS variable theming**: Already doing this — extend the token system
3. **Compound component pattern**: Radix's `Root`/`Trigger`/`Content` pattern is already in use — standardize
4. **New unified Radix import**: Migrate from `@radix-ui/react-dialog` to `radix-ui` (new 2026 unified package)

### 3.4 Why NOT Motion/Framer Motion

| Concern | Explanation |
|---|---|
| Bundle cost | Even with `LazyMotion` + `m`, minimum 15kb for animations CSS can do for free |
| PWA budget | Every kb matters for installable PWAs on slow connections |
| CSS can do it | View transitions, `@keyframes`, `transition`, `backdrop-filter` — all GPU-accelerated |
| auto-animate | 3kb for enter/exit animations covers 90% of needs |
| Houdini + WAAPI | Browser-native animation API covers scroll-driven, timeline, and spring animations without libraries |

**Animation approach: CSS-first, auto-animate for lists, WAAPI `useAnimate` (mini, 2.3kb) if imperative needed.**

---

## 4. PWA-First Architecture

### 4.1 Service Worker (vite-plugin-pwa)

```typescript
// vite.config.ts — PWA configuration
VitePWA({
  registerType: 'prompt',           // Manual update via toast
  includeAssets: ['favicon.svg'],
  manifest: {
    name: 'goloom',
    short_name: 'goloom',
    description: 'Social scheduling for teams',
    display: 'standalone',
    background_color: '#000000',    // Dark default
    theme_color: '#000000',         // Updated dynamically via JS
    orientation: 'portrait-primary',
    categories: ['social', 'productivity'],
    icons: [
      { src: '/icon-192.svg', sizes: '192x192', type: 'image/svg+xml', purpose: 'any' },
      { src: '/icon-512.svg', sizes: '512x512', type: 'image/svg+xml', purpose: 'any maskable' },
    ],
    screenshots: [                  // For app store listing
      // TODO: Add after redesign
    ],
  },
  workbox: {
    globPatterns: ['**/*.{js,css,html,ico,png,svg}'],
    runtimeCaching: [
      {
        urlPattern: /^\/v1\/teams\/[\w-]+\/(posts|accounts|analytics)/i,
        handler: 'NetworkFirst',
        options: {
          cacheName: 'api-data',
          expiration: { maxEntries: 200, maxAgeSeconds: 60 * 60 * 24 * 7 }, // 7 days
        },
      },
      {
        urlPattern: /^\/v1\/(me|auth)/i,
        handler: 'NetworkOnly',      // Never cache auth
      },
    ],
  },
})
```

### 4.2 Manifest Enhancements

- **Dynamic theme_color**: Already implemented via `meta[name="theme-color"]` — keep
- **Screenshots**: Add app screenshots to manifest after redesign for "install promotion"
- **Shortcuts**: Register app shortcuts for "New Post", "Dashboard", "Calendar"
- **Share target**: Register as share target for media sharing

### 4.3 Update Prompt Component

```tsx
// Inspired by vitepwa ReloadPrompt pattern
// Sonner toast: "New version available" → [Reload]
// Sonner toast: "Ready to work offline" → [Dismiss]
```

### 4.4 Offline Page

A minimal offline fallback page served by the service worker when the app is not cached yet.

---

## 5. Visual Language Overhaul

### 5.1 Design Personality

```
┌─────────────────────────────────────────┐
│  Brand: goloom                          │
│  Personality: Clean · Tool-like · Warm  │
│  Inspiration: Linear · Notion · Arc     │
│  Vibe: Professional but friendly         │
│  Gradient mark: 3 stacked rounded rects │
│    (blue → purple → orange)             │
└─────────────────────────────────────────┘
```

### 5.2 Typography Refinement

```css
/* Current */
--font-sans: "Geist", "Inter", ui-sans-serif, system-ui, sans-serif;
--font-mono: "Geist Mono", "JetBrains Mono", ui-monospace, monospace;

/* Keep — system font stack means zero font downloads */
/* But add explicit weights used: */
h1 { font-weight: 750; }  /* Extra-bold for headings */
h2 { font-weight: 650; }
h3 { font-weight: 600; }
.body { font-weight: 450; }
```

**No custom font files to download** — system fonts only, self-hosted compatible.

### 5.3 Color Palette Refinement

Current dark theme is near-black (`#000`, `#0a0a0a`). Evolve to:

```css
[data-theme="dark"] {
  /* Canvas remains black for OLED */
  --canvas: #000000;

  /* Surfaces get subtle warmth instead of pure grey */
  --surface: #0c0c0c;           /* slightly warmer than #0a0a0a */
  --surface-raised: #141414;    /* warmer */
  --surface-overlay: #1e1e1e;   /* for modals, dropdowns */
  --surface-muted: #111111;

  /* Accent: use the brand's purple gradient midpoint */
  --accent: #8b5cf6;
  --accent-lighter: #a78bfa;
  --accent-darker: #7c3aed;

  /* Extended semantic colors */
  --info: #3b82f6;
  --info-soft: rgba(59, 130, 246, 0.12);

  /* Keep existing: success, warning, danger */
}
```

```css
[data-theme="light"] {
  --canvas: #f5f5f0;            /* Slightly warm off-white */
  --surface: #ffffff;
  --surface-raised: #f0efec;
  --surface-overlay: #ffffff;
  --surface-muted: #faf9f7;

  --accent: #7c3aed;            /* Same purple, darker for light bg */
}
```

### 5.4 Mesh Background Enhancement

Current mesh gradients use `violet` and `blue`. Evolve to use the full brand palette:

```css
[data-theme="dark"] .mesh-bg::before {
  /* Purple → pink gradient orb, top-left */
  background: radial-gradient(circle, var(--accent) 0%, var(--accent-lighter) 30%, transparent 70%);
  opacity: 0.10;
}
[data-theme="dark"] .mesh-bg::after {
  /* Blue → cyan gradient orb, bottom-right */
  background: radial-gradient(circle, #38bdf8 0%, #2dd4bf 40%, transparent 70%);
  opacity: 0.08;
}
/* Add a third subtle orb for depth on desktop */
@media (min-width: 1024px) {
  .mesh-bg .orb-center {
    background: radial-gradient(circle, var(--accent-darker) 0%, transparent 60%);
    opacity: 0.06;
  }
}
```

### 5.5 Glassmorphism Refinement

```css
.glass-panel {
  background: var(--surface-raised);
  backdrop-filter: blur(var(--glass-blur));
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-sm);
  /* Add transition for hover states */
  transition: all var(--transition-fast);
}
.glass-panel:hover {
  border-color: var(--border-strong);
  box-shadow: var(--shadow);
}
```

---

## 6. Design Token System Expansion

### 6.1 Current Tokens (Good — Expand)

| Token | Status | Action |
|---|---|---|
| `--font-sans`, `--font-mono` | ✅ Keep | Already good |
| `--space-{1,2,3,4,6,8,12}` | ✅ Keep | Add `--space-16`, `--space-20` |
| `--radius-{sm,md,lg,xl,full}` | ✅ Keep | Already good |
| `--ease-out`, `--transition-{fast,normal}` | ✅ Keep | Add `--transition-slow` |
| `--glass-blur` | ✅ Keep | Already good |
| `--canvas`, `--surface-*`, `--border*` | ✅ Keep | Expand surface variants |
| `--text`, `--text-soft`, `--text-dim` | ✅ Keep | Add `--text-inverse` |
| `--primary`, `--primary-foreground` | ✅ Keep | Good |
| `--accent`, `--accent-soft` | ✅ Keep | Add `--accent-lighter`, `--accent-darker` |
| `--success`, `--warning`, `--danger` | ✅ Keep | Add `--info`, `--info-soft` |
| `--shadow`, `--shadow-sm` | ✅ Keep | Add `--shadow-lg`, `--shadow-xl` |

### 6.2 New Tokens

```css
:root {
  /* Extended spacing */
  --space-16: 4rem;
  --space-20: 5rem;

  /* Extended colors */
  --info: #3b82f6;
  --info-soft: rgba(59, 130, 246, 0.12);

  /* Shadows for depth hierarchy */
  --shadow-lg: 0 16px 48px rgba(0, 0, 0, 0.5);
  --shadow-xl: 0 24px 64px rgba(0, 0, 0, 0.6);

  /* Transition durations */
  --transition-slow: 400ms var(--ease-out);

  /* Z-index scale (avoids magic numbers) */
  --z-base: 0;
  --z-dropdown: 50;
  --z-sticky: 100;
  --z-modal-backdrop: 100;
  --z-modal: 110;
  --z-toast: 200;

  /* Touch targets (PWA) */
  --touch-target: 44px;

  /* Safe areas */
  --safe-area-top: env(safe-area-inset-top, 0px);
  --safe-area-bottom: env(safe-area-inset-bottom, 0px);
}
```

### 6.3 Enforcing Token Usage

**Rule:** All `color`, `background`, `border`, `box-shadow`, `border-radius`, `padding`, `margin`, `gap`, `font-size` values MUST use CSS variables. Zero inline style props after redesign.

---

## 7. Component Architecture

### 7.1 Directory Structure

```
frontend/src/components/
├── ui/                          # shadcn-style atomic components
│   ├── Button.tsx               # Unified button with variants
│   ├── Dialog.tsx               # Radix dialog wrapper
│   ├── DropdownMenu.tsx         # Radix dropdown wrapper
│   ├── Input.tsx                # Styled input with label/slots
│   ├── Select.tsx               # Radix select wrapper
│   ├── Tabs.tsx                 # Radix tabs wrapper
│   ├── Toast.tsx                # Sonner toast export (thin wrapper)
│   ├── Skeleton.tsx             # Loading skeleton component
│   ├── EmptyState.tsx           # Empty state with illustration + CTA
│   ├── Command.tsx              # Radix command palette wrapper
│   ├── Badge.tsx                # Pill/badge component
│   ├── Card.tsx                 # Card primitives (Header, Content, Footer)
│   └── Avatar.tsx               # Unified avatar with fallback
│
├── Shell/                       # App shell (keep existing)
│   ├── AppShell.tsx             # Refactored: no inline styles
│   ├── Sidebar.tsx              # Refactored: no inline styles
│   ├── MobileNav.tsx            # Refactored: bottom nav + drawer
│   └── NavItems.tsx             # Keep, but consolidate icons
│
├── Composer/                    # Keep as-is, refactor inline styles
├── Media/                       # Keep as-is
├── post/                        # Keep as-is
├── auth/                        # Keep as-is, refactor inline styles
├── brand/                       # Keep as-is
└── settings/                    # Keep as-is
```

### 7.2 New UI Components

#### 7.2.1 Button (Unified)

```tsx
// Before: scattered <button className="btn btn--primary"> + <button className="button button--prominent">
// After: one <Button variant="primary" size="md" icon={<Plus />}>
//
// Variants: primary | secondary | ghost | danger | outline
// Sizes: sm | md | lg
// States: loading | disabled
```

**Consolidates:** `btn`, `button`, `btn--primary`, `btn--ghost`, `button--prominent`, `button--secondary` into one component.

#### 7.2.2 Skeleton

```tsx
// Loading placeholder that matches the shape of the content
<Skeleton className="h-8 w-48" />        // Title
<Skeleton className="h-24 w-full" />      // Card
<Skeleton className="h-12 w-12 rounded-full" /> // Avatar
```

Three variants: `text` (single line), `card` (rectangle with border-radius), `circle` (avatar).

#### 7.2.3 EmptyState

```tsx
<EmptyState
  icon={<Calendar />}
  title="No upcoming posts"
  description="Create your first post to start building your publishing timeline."
  action={<Button>Create post</Button>}
/>
```

**Role:** Replace all `<p className="hint">` empty states with visual + CTA.

#### 7.2.4 Command Palette (Ctrl+K)

```tsx
// Search/palette for navigating sections, teams, accounts
// Uses Radix Dialog as base
// Trigger: Cmd+K / Ctrl+K desktop, long-press More on mobile
```

**Sections to search:** All navigation items, teams by name, accounts by name, posts by title.

---

## 8. Mobile Navigation & Gesture System

### 8.1 Bottom Navigation Redesign

```
┌─────────────────────────────────┐
│  ●                          ○   │  ← Status bar (safe area)
├─────────────────────────────────┤
│                                 │
│         Main Content            │
│                                 │
├─────────────────────────────────┤
│  🏠  📅  ➕  🖼️  ☰             │  ← Bottom nav
│ Home  Cal  Post  Media  More    │
└─────────────────────────────────┘
```

**Changes from current:**
- Replace `<Menu>` icon text "More" with clearer label
- Add active indicator (dot) for current section
- Pull-to-refresh on scrollable content areas (Dashboard, Schedule, Archive)
- **Swipe between main tabs** (Home ↔ Calendar) on mobile

### 8.2 Pull-to-Refresh

Implemented via CSS `overscroll-behavior: contain` + a custom hook:

```tsx
function usePullToRefresh(onRefresh: () => Promise<void>) {
  // Tracks touch events, shows spinner at top threshold
  // Triggers onRefresh when pull exceeds 80px and released
}
```

### 8.3 Swipe Gestures

For the timeline/calendar view on mobile:
- **Left swipe** on post card: reveals "Edit" + "Delete" actions
- **Swipe between tabs** on dashboard: Home ↔ Calendar

### 8.4 Bottom Sheet for Mobile

Mobile composer and drawer use Radix Dialog with `data-side="bottom"` already. Refine:

```css
/* Animated slide-up with spring easing */
@keyframes sheet-up {
  from { transform: translateY(100%); }
  to { transform: translateY(0); }
}
```

---

## 9. Desktop Layout Refinements

### 9.1 Current Layout

```
┌──────────────┬───────────────────┬────────────┐
│   Sidebar    │    Main Content   │  Preview   │
│   (240px)    │      (1fr)        │  (380px)   │
│              │                   │            │
│  Logo        │  Page header      │  Post      │
│  Team menu   │  Status banner    │  preview   │
│  New Post    │  View content     │            │
│  Navigation  │                   │            │
│  Sign out    │                   │            │
└──────────────┴───────────────────┴────────────┘
```

**Refinements:**
- Sidebar: Add collapsed state (`width: 64px` — icons only, expand on hover)
- Main content: Better max-width handling for readability (`max-width: 1200px` within 1fr)
- Preview column: Persistent scroll position when switching posts

### 9.2 Sidebar Collapsed State

```css
.sidebar {
  width: 240px;
  transition: width var(--transition-normal);
}
.sidebar--collapsed {
  width: 64px;
}
.sidebar--collapsed .sidebar__label,
.sidebar--collapsed .sidebar__team-name,
.sidebar--collapsed .btn__text {
  display: none;
}
```

Toggle via shortcut `Cmd+B` or a collapse button at sidebar bottom.

### 9.3 Page Header Unified Component

Current: Each view has its own header rendered inline. Unify:

```tsx
<PageHeader
  eyebrow={section === 'dashboard' ? 'Workspace' : 'Social publishing'}
  title={SECTION_HEADINGS[section]}
  actions={
    <ThemeToggle />,
    <SearchTrigger />,
  }
/>
```

---

## 10. Animation & Motion System

### 10.1 Philosophy

**CSS-first, zero JS animation library.** Use browser-native tools:

| Need | Solution | Bundle cost |
|---|---|---|
| Page transitions | CSS `@keyframes` | 0kb |
| List enter/exit | `@formkit/auto-animate` (3kb) | 3kb |
| Micro-interactions (hover, tap) | CSS `transition` | 0kb |
| Scroll-driven animations | CSS `animation-timeline: scroll()` | 0kb |
| Gesture animations (swipe) | WAAPI via `useAnimate` mini | 2.3kb (if needed) |
| Spring physics | CSS `linear()` easing | 0kb |

### 10.2 View Transitions

```css
/* Enhanced from current view-fade-in */
@keyframes view-enter {
  from { opacity: 0; transform: translateY(12px) scale(0.98); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}

.app-main {
  animation: view-enter 350ms var(--ease-out);
}
```

### 10.3 Micro-interactions

```css
/* Button press feedback */
.btn:active {
  transform: scale(0.97);
  transition: transform 100ms var(--ease-out);
}

/* Card hover */
.post-card {
  transition: transform var(--transition-fast),
              border-color var(--transition-fast),
              box-shadow var(--transition-fast);
}
.post-card:hover {
  transform: translateY(-2px);
  border-color: var(--border-strong);
  box-shadow: var(--shadow-sm);
}

/* Loading shimmer */
@keyframes shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}
.skeleton {
  background: linear-gradient(
    90deg,
    var(--surface-muted) 25%,
    var(--surface-overlay) 50%,
    var(--surface-muted) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}
```

### 10.4 auto-animate Integration

```tsx
import { useAutoAnimate } from '@formkit/auto-animate/react'

function PostList() {
  const [parent] = useAutoAnimate()
  return <div ref={parent}>
    {posts.map(post => <PostCard key={post.id} ... />)}
  </div>
}
// Zero-config enter/exit animations for list changes
```

Targets: Dashboard upcoming posts, Schedule timeline, Media Library grid.

---

## 11. Data Layer & Offline Strategy

### 11.1 Why TanStack Query

Current state: All data flows through `useState` + `useEffect` in `App.tsx` — a single monolithic component with 100+ state variables. This makes it impossible to:
- Cache data across navigations (refetches everything on section change)
- Show stale data while offline
- Retry failed requests
- Persist anything beyond a page reload

**TanStack Query solves:**
- Per-resource caching with `staleTime` / `gcTime`
- Offline persistence via IndexedDB
- Automatic background refetching
- Optimistic updates for mutations
- Request deduplication

### 11.2 Migration Strategy

**Phase 1 — Wrap existing (non-breaking):**
```tsx
// Add QueryClientProvider at root level
// Keep all existing useState/useEffect as-is
// Start wrapping data fetches with useQuery one-by-one

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 2,         // 2min — fresh enough
      gcTime: 1000 * 60 * 60 * 24 * 7,  // 7 days in cache
      networkMode: 'offlineFirst',
      retry: 2,
      refetchOnWindowFocus: false,       // PWA — don't refetch on focus
    },
  },
})
```

**Phase 2 — Persistence:**
```tsx
// PersistQueryClientProvider wraps QueryClientProvider
// Uses idb-keyval for IndexedDB storage

const persister = createAsyncStoragePersister({
  storage: {
    getItem: get,         // idb-keyval
    setItem: set,         // idb-keyval
    removeItem: del,      // idb-keyval
  },
})
```

**Phase 3 — Decompose App.tsx:**
- Move data fetching into custom hooks: `useTeams()`, `usePosts(teamId)`, `useAccounts(teamId)`
- App.tsx becomes orchestration-only (section state, composer state)
- Each view fetches its own data via hooks

### 11.3 Initial Implementation (without full refactor)

For the redesign, start small:
1. Install TanStack Query + idb-keyval
2. Wrap `loadDashboard()` as a query with key `['dashboard', teamId]`
3. Add persister for offline resilience
4. Migrate analytics and media library queries next
5. Leave App.tsx decomposition for Phase 3

---

## 12. Light & Dark Theming

### 12.1 Current (Good Foundation)

- `[data-theme="dark/light"]` on `.app-shell`
- All colors are CSS variables
- System preference detection works
- Manual toggle via Sun/Moon button

### 12.2 Refinements

**Smoother transition:**
```css
.app-shell {
  transition: background-color var(--transition-normal),
              color var(--transition-normal);
}
* {
  /* Apply to all themed elements */
  transition: background-color var(--transition-fast),
              border-color var(--transition-fast),
              color var(--transition-fast);
}
```

**Theme toggle in sidebar (desktop):**
Move the Sun/Moon button from floating header position to sidebar footer, next to Sign Out.

**Color scheme for components that ignore tokens:**
The `SocialPreview` component currently uses hardcoded `#fff`, `#e5e7eb`, etc. for preview fidelity. Keep this — social previews should match platform appearance, not app theme.

### 12.3 High Contrast Mode

Add `[data-theme="dark"][data-contrast="high"]` variant for accessibility:
```css
[data-theme="dark"][data-contrast="high"] {
  --text: #ffffff;
  --text-soft: #e4e4e7;
  --border: rgba(255, 255, 255, 0.2);
  --border-strong: rgba(255, 255, 255, 0.35);
}
```

Toggle in Settings → UI → Contrast.

---

## 13. View-by-View Redesign Specification

### 13.1 Auth Screen (First Impression)

**Current:** Centered card with logo, title, token input or OIDC button. Text-based, minimal.

**Redesign:**
- Full-screen with animated brand background (3 stacked gradient rects matching favicon)
- Logo centered, larger, with subtle float animation
- Card is glass-panel with more generous padding
- Input is visually prominent with clear label
- OIDC button is primary action, token input is secondary (folded under "or use a token")
- Error states inline with shaking animation

```
┌────────────────────────────────────┐
│                                    │
│      ┌────────────────────┐        │
│      │                    │        │
│      │   ██  ██  ██       │        │  Logo (animated)
│      │   goloom           │        │
│      │   Social scheduling│        │
│      │                    │        │
│      │   ┌──────────────┐ │        │
│      │   │ Continue with│ │        │  Primary CTA
│      │   │ OpenID Connect│ │        │
│      │   └──────────────┘ │        │
│      │   or use a token  │ │        │
│      │   ┌──────────────┐ │        │
│      │   │ Bearer token │ │        │  Secondary input
│      │   │ [············]│ │        │
│      │   └──────────────┘ │        │
│      │                    │        │
│      └────────────────────┘        │
│                                    │
└────────────────────────────────────┘
```

### 13.2 Dashboard

**Current:** Grid of 3 panels (Performance, Account health, Upcoming posts). Text placeholders for loading.

**Redesign:**
- **Welcome header**: Greeting with team name, brief stats row ("12 posts this week · 89% scheduled" )
- **Engagement sparklines**: Same 3, but with proper skeleton loading (animated shimmer cards)
- **Account health**: Keep same grid, add color-coded status dots and reconnect button inline
- **Upcoming posts**: Add auto-animate for list changes, show 3 posts expanded by default with "Show all" toggle
- **Quick actions bar**: "New post" button, "Open calendar", "View analytics" as icon buttons
- **Empty states**: Illustrated empty state when no accounts/posts configured

### 13.3 Schedule / Timeline

**Current:** Month-grouped list of PostCards. Preview column on right.

**Redesign:**
- **Compact timeline view**: Date headers with post density indicator
- **PostCard improvements**: Show first 2 lines of content, platform icons, status badge
- **Swipe on mobile**: Reveal Edit/Delete on horizontal swipe
- **Drag handle on desktop**: Subtle grip indicator for reordering (future feature)
- **Filter bar**: Filter by platform (Mastodon/Bluesky/Friendica), status (draft/scheduled)
- **Infinite scroll**: Load more as user scrolls down (or pagination buttons)

### 13.4 Content Calendar

**Current:** Month grid with post chips. Drag-and-drop. Separate list view for mobile.

**Redesign:**
- **Consistent with Schedule**: Calendar is the visual overview, Schedule is the list
- **Month navigation**: Smoother animation on month change
- **Post chips**: Show time and truncated title. Color-coded by platform.
- **Drag-and-drop**: Keep, add visual drop target highlight (already partially done)
- **Today indicator**: Bold outline on current day cell

### 13.5 Post Composer

**Current:** Full modal with edit/preview columns. Mobile has toggle tabs.

**Redesign:**
- **Mobile: Bottom sheet** instead of full modal (current is already modal — refine)
- **Desktop: Slide-in panel** from right instead of modal overlay (optional — modal is fine)
- **Account selector**: Circular avatars with checkmarks (keep current pattern, refine styling)
- **Character count**: Circular progress indicator instead of text ("142/500")
- **Preview**: Keep real-time preview but add loading state for media
- **Schedule insights**: Keep as-is — well designed already
- **Skeleton while loading**: If editing an existing post, show skeleton while versions load

### 13.6 Analytics

**Current:** Multiple sections with charts. Recharts for all visualizations.

**Redesign:**
- **Summary cards**: Animated number counters on mount
- **Chart consistency**: All charts share same tooltip style, axis styling, color scheme
- **Time range selector**: 7d / 30d / 90d tabs at top (currently hardcoded 30d)
- **Responsive**: Charts stack vertically on mobile (currently side-by-side)
- **Loading state**: Skeleton cards matching chart dimensions

### 13.7 Media Library

**Current:** Simple grid with upload/delete. Plain loading.

**Redesign:**
- **Grid improvements**: Responsive columns with consistent aspect ratios
- **Upload drop zone**: Visually prominent dashed area with drag-and-drop support
- **Loading grid**: Skeleton tiles matching media thumb dimensions
- **Lightbox**: Click to preview media in full-size overlay
- **Empty state**: Illustrated empty with upload CTA

### 13.8 Settings, Teams, Accounts, Admin

**Current:** Functional but visually basic. Glass-panel containers with inline forms.

**Redesign:**
- **Unified form styles**: All inputs use the same `Input` component with consistent labels
- **Section grouping**: Cards with clear headers and spacing hierarchy
- **Saving state**: Button loading spinner + success toast instead of inline status banner
- **Danger zone**: Red-bordered section for destructive actions (delete, revoke)

---

## 14. Implementation Phases

### Phase 0: Foundation (Week 1)

**Goal:** Install new dependencies, establish component library patterns, no visual changes yet.

| Task | Details |
|---|---|
| Install `vite-plugin-pwa` | Configure SW with `prompt` strategy, generate manifest |
| Install `@tanstack/react-query` + persister + `idb-keyval` | Set up `QueryClientProvider` at root |
| Install `sonner` | Set up `Toaster` component at root |
| Create `frontend/src/components/ui/` | Begin with Button, Input, Skeleton, EmptyState |
| Adopt unified `radix-ui` package | Migrate from `@radix-ui/react-*` to single `radix-ui` |
| Remove `icons.tsx` | Replace all custom SVG with Lucide equivalents |

### Phase 1: PWA Shell (Week 2)

**Goal:** Service worker live, offline caching working, toast system active.

| Task | Details |
|---|---|
| Configure `vite-plugin-pwa` | `generateSW` with API runtime caching |
| Add `ReloadPrompt` / update toast | Sonner toast for "new version available" |
| Add dynamic theme-color meta | Already done — verify PWA install prompt |
| Add PWA shortcuts and share target | Manifest enhancements |
| Add initial TanStack Query wrapper | Wrap `loadDashboard` — test offline caching |
| Add offline fallback page | Minimal "connect to internet" page |

### Phase 2: Visual Overhaul (Weeks 3-4)

**Goal:** New color palette, animation system, component library.

| Task | Details |
|---|---|
| Expand design tokens | New colors, spacing, shadows, z-index scale |
| Refine color palette | Warm dark/light surfaces, brand-consistent |
| Implement button unification | Replace all `btn`, `button` variants with single `Button` |
| Implement Skeleton component | Shimmer animation, 3 variants |
| Implement EmptyState component | Icon + title + description + action |
| Add auto-animate to list views | Dashboard, Schedule, Media Library |
| Remove all inline `style={{}}` | Replace with CSS classes and tokens |
| Add micro-interactions | Card hover, button press, input focus |

### Phase 3: Mobile UX (Week 5)

**Goal:** Gesture navigation, pull-to-refresh, polished bottom nav.

| Task | Details |
|---|---|
| Add pull-to-refresh hook | Dashboard, Schedule, Archive |
| Add swipe-to-reveal on post cards | Edit/Delete actions |
| Refine bottom nav | Active indicator, better labels |
| Add tablet breakpoint | Sidebar collapses at 768-1023px |
| Bottom sheet animation refinement | Spring easing for drawer |

### Phase 4: View-by-View Polish (Weeks 6-7)

**Goal:** Each view redesigned per Section 13 spec.

| Task | Details |
|---|---|
| Auth screen redesign | Animated background, visual hierarchy |
| Dashboard refinements | Welcome header, quick actions, shimmer skeletons |
| Schedule/Archive refinements | Skeleton cards, filter bar, infinite scroll |
| Composer refinements | Bottom sheet on mobile, circular char count |
| Analytics refinements | Animated counters, time range selector |
| Media Library refinements | Lightbox, drag-drop upload, skeleton grid |
| Settings/Admin/Teams refinements | Unified form inputs, section cards |

### Phase 5: Command Palette & Power User (Week 8)

**Goal:** Productivity features.

| Task | Details |
|---|---|
| Command palette component | Radix Dialog + cmdk pattern |
| Ctrl+K / Cmd+K trigger | Search sections, teams, accounts, posts |
| Sidebar collapse on desktop | Icon-only mode, Cmd+B toggle |
| Keyboard shortcuts | Show in tooltip on hover |

### Phase 6: Polish & QA (Week 9)

**Goal:** Accessibility audit, performance budget verification, edge cases.

| Task | Details |
|---|---|
| Accessibility audit | Tab order, focus rings, screen reader testing |
| Performance audit | Bundle size, CLS, animation frame rate |
| Touch target audit | All interactive elements ≥ 44px |
| Dark/light mode audit | Every view checked in both themes |
| Build + deploy test | Verify Go binary embedding, no CDN requests |

---

## 15. Quality Standards & Verification

### 15.1 Code Quality

| Standard | Verification |
|---|---|
| No inline `style={{}}` props | `grep -r 'style={{' frontend/src/` should return 0 |
| All colors from CSS variables | Review against token list |
| No `as any` or `@ts-ignore` | LSP diagnostics clean |
| Component uses Radix for a11y | Review component implementation |
| Loading state present | Every data-driven view has Skeleton or spinner |
| Empty state present | Every data-driven view has EmptyState component |

### 15.2 PWA Standards

| Standard | Verification |
|---|---|
| Install prompt fires | Lighthouse PWA audit "installable" |
| Offline page loads | Test with DevTools offline toggle |
| API data cached for 7 days | Workbox runtimeCaching configured |
| Theme-color matches current theme | Already verified |
| Touch targets ≥ 44px | Interactive elements audit |

### 15.3 Performance Budget

| Metric | Target |
|---|---|
| Initial JS bundle (gzip) | < 100kb |
| TanStack Query cache hydration | < 50ms |
| View transition animation | 350ms max |
| First Contentful Paint | < 1.5s |
| Time to Interactive | < 3s |

### 15.4 Accessibility

| Standard | Target |
|---|---|
| Focus management | All modals trap focus (Radix default) |
| Color contrast | WCAG AA minimum (4.5:1 text, 3:1 large text) |
| Keyboard navigation | All interactive elements reachable via Tab |
| Screen reader | ARIA labels on icons, proper heading hierarchy |
| Reduced motion | `prefers-reduced-motion` respected |

---

## Appendix A: File Change Inventory

### New Files

```
frontend/src/components/ui/Button.tsx
frontend/src/components/ui/Dialog.tsx
frontend/src/components/ui/Input.tsx
frontend/src/components/ui/Select.tsx
frontend/src/components/ui/Tabs.tsx
frontend/src/components/ui/Skeleton.tsx
frontend/src/components/ui/EmptyState.tsx
frontend/src/components/ui/Badge.tsx
frontend/src/components/ui/Toast.tsx
frontend/src/components/ui/Avatar.tsx
frontend/src/components/Shell/CommandPalette.tsx
frontend/src/hooks/usePullToRefresh.ts
frontend/src/pwa/ReloadPrompt.tsx
frontend/public/sw.js              (generated by vite-plugin-pwa)
frontend/public/offline.html
```

### Modified Files

```
frontend/vite.config.ts              ← Add vite-plugin-pwa
frontend/package.json                ← Add dependencies
frontend/src/index.css               ← Expand design tokens, new animations
frontend/src/main.tsx                ← Add providers (QueryClient, Toaster)
frontend/src/App.tsx                 ← Decompose data fetching into hooks
frontend/src/components/Shell/*      ← Remove inline styles, add collapse
frontend/src/components/auth/*       ← Refactor styling
frontend/src/components/post/*       ← Refactor styling
frontend/src/views/*                 ← Add skeletons, empty states, remove inline styles
frontend/public/manifest.json        ← Enhanced PWA manifest
```

### Removed Files

```
frontend/src/icons.tsx               ← Replaced by Lucide
```

---

## Appendix B: Dependency Decision Log

| Decision | Choice | Rejected Alternatives | Rationale |
|---|---|---|---|
| Animation | CSS + auto-animate | Motion/Framer Motion (15-30kb) | Bundle discipline for PWA |
| Toast | Sonner (2.5kb) | react-hot-toast (4kb), react-toastify (17kb) | Smallest, most accessible, shadcn default |
| Data fetching | TanStack Query v5 (12kb) | RTK (13kb + Redux 9kb), SWR (8kb) | Best offline support, persistence API |
| Component patterns | shadcn-style (copy-paste) | Tailwind + shadcn CLI | Avoid CSS bloat for self-hosted binary |
| PWA | vite-plugin-pwa | Manual workbox | Zero-config, Vite-native |
| Icons | Lucide exclusively | Custom SVG + Lucide mixed | Single source of truth |

---

## Appendix C: Migration Strategy

### To Avoid Breaking Changes

1. **CSS tokens**: Additive only — existing variables remain. New views use new tokens.
2. **Component library**: New `ui/` components added alongside old ones. Views migrate one by one.
3. **TanStack Query**: Wraps existing `loadDashboard()` — no immediate rewrite needed.
4. **vite-plugin-pwa**: Additive — SW registers in background, app functions identically without it.
5. **Lucide migration**: `grep` for `Icon` usage in `icons.tsx`, replace one file at a time.

### Rollback Plan

If any phase causes regressions:
- `git revert` the commit for that specific phase
- The SW can be unregistered via Application > Service Workers in DevTools
- Old icons.tsx is always one git checkout away
