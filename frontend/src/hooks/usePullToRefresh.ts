import { useCallback, useEffect, useRef, useState } from 'react'

export type PullPhase = 'idle' | 'pulling' | 'threshold' | 'refreshing'

export interface UsePullToRefreshOptions {
  onRefresh: () => void | Promise<void>
  threshold?: number
  disabled?: boolean
}

export interface UsePullToRefreshReturn {
  pullDistance: number
  isRefreshing: boolean
  pullPhase: PullPhase
  containerRef: React.RefCallback<HTMLElement | null>
}

export function usePullToRefresh({
  onRefresh,
  threshold = 80,
  disabled = false,
}: UsePullToRefreshOptions): UsePullToRefreshReturn {
  const [pullDistance, setPullDistance] = useState(0)
  const [pullPhase, setPullPhase] = useState<PullPhase>('idle')

  const containerElement = useRef<HTMLElement | null>(null)
  const touchStartY = useRef(0)
  const isPulling = useRef(false)
  const refreshPromise = useRef<Promise<void> | null>(null)
  const pullDistanceRef = useRef(0)

  const isRefreshing = pullPhase === 'refreshing'

  const containerRef = useCallback((node: HTMLElement | null) => {
    containerElement.current = node
  }, [])

  const triggerRefresh = useCallback(() => {
    if (refreshPromise.current) return
    setPullPhase('refreshing')
    setPullDistance(0)
    const result = onRefresh()
    refreshPromise.current = Promise.resolve(result)
    refreshPromise.current.finally(() => {
      refreshPromise.current = null
      setPullPhase('idle')
      setPullDistance(0)
    })
  }, [onRefresh])

  useEffect(() => {
    const el = containerElement.current
    if (!el || disabled) return

    const handleTouchStart = (e: TouchEvent) => {
      if (refreshPromise.current) return
      if (el.scrollTop > 0) return
      if (e.touches.length !== 1) return

      touchStartY.current = e.touches[0].clientY
      isPulling.current = true
      setPullPhase('pulling')
    }

    const handleTouchMove = (e: TouchEvent) => {
      if (!isPulling.current || refreshPromise.current) return
      if (e.touches.length !== 1) return

      const delta = e.touches[0].clientY - touchStartY.current
      if (delta <= 0) {
        setPullDistance(0)
        pullDistanceRef.current = 0
        setPullPhase('idle')
        isPulling.current = false
        return
      }

      const resisted = Math.round(delta * 0.45)
      pullDistanceRef.current = resisted
      setPullDistance(resisted)
      setPullPhase(resisted >= threshold ? 'threshold' : 'pulling')
    }

    const handleTouchEnd = () => {
      if (!isPulling.current || refreshPromise.current) return

      isPulling.current = false

      if (pullDistanceRef.current >= threshold) {
        triggerRefresh()
      } else {
        setPullDistance(0)
        pullDistanceRef.current = 0
        setPullPhase('idle')
      }
    }

    const handleTouchCancel = () => {
      isPulling.current = false
      pullDistanceRef.current = 0
      setPullDistance(0)
      setPullPhase('idle')
    }

    el.addEventListener('touchstart', handleTouchStart, { passive: true })
    el.addEventListener('touchmove', handleTouchMove, { passive: true })
    el.addEventListener('touchend', handleTouchEnd)
    el.addEventListener('touchcancel', handleTouchCancel)

    return () => {
      el.removeEventListener('touchstart', handleTouchStart)
      el.removeEventListener('touchmove', handleTouchMove)
      el.removeEventListener('touchend', handleTouchEnd)
      el.removeEventListener('touchcancel', handleTouchCancel)
    }
  }, [disabled, threshold, triggerRefresh])

  return { pullDistance, isRefreshing, pullPhase, containerRef }
}
