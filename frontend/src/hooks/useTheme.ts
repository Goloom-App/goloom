import { useEffect, useMemo, useState } from 'react'

/** Tracks the mobile breakpoint (max-width: 900px). */
export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState(() =>
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(max-width: 900px)').matches
      : false,
  )

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return
    }
    const mediaQuery = window.matchMedia('(max-width: 900px)')
    const syncMobile = (event?: MediaQueryListEvent) => {
      setIsMobile(event ? event.matches : mediaQuery.matches)
    }
    syncMobile()
    mediaQuery.addEventListener('change', syncMobile)
    return () => mediaQuery.removeEventListener('change', syncMobile)
  }, [])

  return isMobile
}

/**
 * Resolves the effective theme from the user preference ('system' follows the
 * OS) and mirrors it onto <html> (data-theme + PWA theme-color) so portals
 * and the browser chrome stay in sync.
 */
export function useResolvedTheme(colorScheme: string): 'dark' | 'light' {
  const [systemIsDark, setSystemIsDark] = useState(() =>
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : true,
  )

  const resolvedTheme = useMemo((): 'dark' | 'light' => {
    if (colorScheme === 'dark') {
      return 'dark'
    }
    if (colorScheme === 'light') {
      return 'light'
    }
    return systemIsDark ? 'dark' : 'light'
  }, [colorScheme, systemIsDark])

  useEffect(() => {
    if (colorScheme !== 'system') {
      return
    }
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const syncTheme = (event?: MediaQueryListEvent) => {
      setSystemIsDark(event ? event.matches : mediaQuery.matches)
    }
    syncTheme()
    mediaQuery.addEventListener('change', syncTheme)
    return () => mediaQuery.removeEventListener('change', syncTheme)
  }, [colorScheme])

  useEffect(() => {
    if (typeof document === 'undefined') {
      return
    }
    const metaTheme = document.querySelector('meta[name="theme-color"]')
    if (metaTheme) {
      metaTheme.setAttribute('content', resolvedTheme === 'dark' ? '#0a0c10' : '#f1f3f6')
    }
    // Sync theme to document element so Radix portals (rendered to body) inherit CSS variables
    document.documentElement.setAttribute('data-theme', resolvedTheme)
  }, [resolvedTheme])

  return resolvedTheme
}
