import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useKnowledgeSources } from '../../hooks/useAI'

// null inherits; [] explicitly uses no additional team knowledge.
export function KnowledgeSourcePicker({ teamId, value, onChange }: {
  teamId: string
  value: string[] | null
  onChange: (ids: string[] | null) => void
}) {
  const { t } = useTranslation()
  const { data: sources, isError, isLoading } = useKnowledgeSources(teamId)
  const [mode, setMode] = useState(value === null ? 'inherit' : value.length ? 'specific' : 'none')
  const missing = sources ? (value ?? []).filter(id => !sources.some(source => source.id === id)) : []
  const toggle = (id: string, checked: boolean) => onChange(checked ? [...(value ?? []), id] : (value ?? []).filter(item => item !== id))
  return <fieldset className="stack stack--sm">
    <legend>{t('knowledgeSelection.label')}</legend>
    <select aria-label={t('knowledgeSelection.label')} data-testid="knowledge-mode" value={mode} onChange={event => {
      const next = event.target.value
      setMode(next)
      onChange(next === 'inherit' ? null : [])
    }}>
      <option value="inherit">{t('knowledgeSelection.inherit')}</option>
      <option value="specific">{t('knowledgeSelection.specific')}</option>
      <option value="none">{t('knowledgeSelection.none')}</option>
    </select>
    <p className="hint">{t('knowledgeSelection.boundary')}</p>
    {isError ? <p role="alert">{t('knowledgeSelection.loadFailed')}</p> : null}
    {mode === 'specific' ? <>
      {isLoading ? <p className="hint">{t('knowledgeSelection.loading')}</p> : null}
      {sources?.map(source => <label key={source.id} className="flex-row">
        <input type="checkbox" checked={value?.includes(source.id) ?? false} onChange={event => toggle(source.id, event.target.checked)} />
        {source.name}
      </label>)}
      {missing.length ? <p role="alert" data-testid="knowledge-missing">{t('knowledgeSelection.missing')}</p> : null}
      {missing.map(id => <label key={id} className="flex-row">
        <input type="checkbox" checked onChange={() => toggle(id, false)} />
        {t('knowledgeSelection.unavailable', { id })}
      </label>)}
    </> : null}
  </fieldset>
}
