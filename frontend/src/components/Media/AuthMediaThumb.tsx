import { useEffect, useRef, useState } from 'react'
import { Icon } from '../../icons'

/**
 * Loads a protected preview URL with Bearer auth and shows it as an inline image.
 */
export function AuthMediaThumb({
  url,
  authHeader,
  alt,
  className,
}: {
  url: string
  authHeader: string
  alt?: string
  className?: string
}) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [error, setError] = useState(false)
  const objectUrlRef = useRef<string | null>(null)

  useEffect(() => {
    let cancelled = false
    objectUrlRef.current = null
    setBlobUrl(null)
    setError(false)

    void (async () => {
      try {
        const headers: HeadersInit = {}
        if (authHeader) {
          headers.Authorization = authHeader
        }
        const response = await fetch(url, { headers })
        if (!response.ok) {
          throw new Error('preview failed')
        }
        const blob = await response.blob()
        const u = URL.createObjectURL(blob)
        if (cancelled) {
          URL.revokeObjectURL(u)
          return
        }
        objectUrlRef.current = u
        setBlobUrl(u)
      } catch {
        if (!cancelled) {
          setError(true)
        }
      }
    })()

    return () => {
      cancelled = true
      if (objectUrlRef.current) {
        URL.revokeObjectURL(objectUrlRef.current)
        objectUrlRef.current = null
      }
    }
  }, [url, authHeader])

  if (error) {
    return (
      <span className={`auth-media-thumb auth-media-thumb--error ${className ?? ''}`} aria-hidden>
        <Icon name="image" />
      </span>
    )
  }

  if (!blobUrl) {
    return (
      <span className={`auth-media-thumb auth-media-thumb--loading ${className ?? ''}`} aria-hidden />
    )
  }

  return <img src={blobUrl} alt={alt ?? ''} className={`auth-media-thumb__img ${className ?? ''}`} loading="lazy" />
}
