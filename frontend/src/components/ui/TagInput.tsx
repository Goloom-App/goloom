import { useState } from 'react'
import { Plus, X } from 'lucide-react'

export interface TagInputProps {
  values: string[]
  onChange: (next: string[]) => void
  placeholder: string
  testId?: string
  /** Hide the inline add button (Enter still works). */
  inlineOnly?: boolean
}

/**
 * Tag/chip input. Adds the trimmed value on Enter or Add. Skips duplicates.
 */
export function TagInput({ values, onChange, placeholder, testId, inlineOnly }: TagInputProps) {
  const [draft, setDraft] = useState('')

  const add = () => {
    const trimmed = draft.trim()
    if (!trimmed) return
    if (values.includes(trimmed)) {
      setDraft('')
      return
    }
    onChange([...values, trimmed])
    setDraft('')
  }

  return (
    <div className="brand-tag-input">
      {values.length > 0 && (
        <div className="brand-tag-input__chips">
          {values.map((value, idx) => (
            <span key={`${value}-${idx}`} className="brand-tag-input__chip">
              <span>{value}</span>
              <button
                type="button"
                className="brand-tag-input__chip-remove"
                aria-label={`${value} entfernen`}
                onClick={() => onChange(values.filter((_, i) => i !== idx))}
              >
                <X size={12} />
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="brand-tag-input__row">
        <input
          data-testid={testId}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={placeholder}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
        />
        {!inlineOnly && (
          <button
            type="button"
            className="btn btn--secondary btn--sm"
            onClick={add}
            disabled={!draft.trim()}
          >
            <Plus size={14} /> Add
          </button>
        )}
      </div>
    </div>
  )
}
