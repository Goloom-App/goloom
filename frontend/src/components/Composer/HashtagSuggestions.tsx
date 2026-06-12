import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { BackendHashtagPerformance, createApiClient } from '../../api'

type Api = ReturnType<typeof createApiClient>

// Mirrors the backend extraction: unicode word characters after '#'.
const usedTagRe = /#([\p{L}\p{N}_]+)/gu
const trailingTagRe = /(^|\s)#([\p{L}\p{N}_]*)$/u

// HashtagSuggestions shows the team's best-performing hashtags as chips below
// the composer body. While the user types a '#token' at the end of the text,
// the chips narrow to matching tags and clicking completes the token.
export function HashtagSuggestions({
  teamId,
  api,
  value,
  onChange,
}: {
  teamId: string
  api: Api
  value: string
  onChange: (next: string) => void
}) {
  const { t } = useTranslation()
  const [tags, setTags] = useState<BackendHashtagPerformance[]>([])

  useEffect(() => {
    if (!teamId) {
      setTags([])
      return
    }
    let cancelled = false
    void api
      .getTeamHashtagPerformance(teamId, { days: 90, limit: 30 })
      .then((res) => {
        if (!cancelled) {
          setTags((res.items ?? []).filter((item) => item.total_engagement > 0))
        }
      })
      .catch(() => {
        if (!cancelled) {
          setTags([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [api, teamId])

  const trailing = value.match(trailingTagRe)
  const prefix = trailing ? trailing[2].toLowerCase() : null

  const visible = useMemo(() => {
    if (tags.length === 0) {
      return []
    }
    const used = new Set<string>()
    for (const m of value.matchAll(usedTagRe)) {
      used.add(m[1].toLowerCase())
    }
    return tags
      .filter((tag) => !used.has(tag.tag) && (prefix === null || prefix === '' || tag.tag.startsWith(prefix)))
      .slice(0, 8)
  }, [prefix, tags, value])

  if (visible.length === 0) {
    return null
  }

  const insert = (tag: BackendHashtagPerformance) => {
    const display = tag.display || tag.tag
    if (trailing) {
      onChange(value.slice(0, value.length - trailing[2].length - 1) + `#${display} `)
      return
    }
    const sep = value === '' || value.endsWith(' ') || value.endsWith('\n') ? '' : ' '
    onChange(`${value}${sep}#${display} `)
  }

  return (
    <div className="hashtag-suggestions">
      <span className="hint">{t('composer.suggestedHashtags')}</span>
      <div className="hashtag-suggestions__chips">
        {visible.map((tag) => (
          <button
            key={tag.tag}
            type="button"
            className="hashtag-suggestions__chip"
            onClick={() => insert(tag)}
            title={t('composer.hashtagChipTitle', {
              uses: tag.uses,
              avg: tag.avg_engagement.toLocaleString(undefined, { maximumFractionDigits: 1 }),
            })}
          >
            #{tag.display || tag.tag}
          </button>
        ))}
      </div>
    </div>
  )
}
