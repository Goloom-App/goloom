import { Icon } from '../../icons'

export type MediaIntegrationHandlers = {
  onRequestUnsplash?: () => void
  onRequestGiphy?: () => void
}

export type MediaSourceTab = 'uploaded' | 'unsplash' | 'giphy'

/**
 * Media workspace: optional source tabs (page layout), grid placeholders, metadata sidebar.
 * Unsplash/Giphy fire optional integration callbacks (e.g. show notice until API keys exist).
 */
export function MediaLibraryPanel({
  selectedLabel,
  integrations,
  layout = 'embedded',
  sourceTab,
  onSourceTabChange,
}: {
  selectedLabel: string
  integrations?: MediaIntegrationHandlers
  layout?: 'embedded' | 'page'
  sourceTab?: MediaSourceTab
  onSourceTabChange?: (tab: MediaSourceTab) => void
}) {
  const showSources = layout === 'page' && sourceTab != null && onSourceTabChange != null

  const sourceHint =
    sourceTab === 'uploaded'
      ? 'Uploads from the composer and future workspace asset APIs will appear here.'
      : sourceTab === 'unsplash'
        ? 'Stock search needs Unsplash API credentials configured for this deployment.'
        : 'GIF search needs Giphy API credentials configured for this deployment.'

  return (
    <div className={`media-library ${layout === 'page' ? 'media-library--page' : ''}`}>
      <div className="media-library__header">
        <p className="eyebrow">Media library</p>
        <p className="hint">{showSources ? `Source: ${sourceTab} · ${selectedLabel}` : `Attach references for ${selectedLabel}`}</p>
      </div>

      {showSources ? (
        <div className="media-library__source-tabs" role="tablist" aria-label="Media source">
          {(['uploaded', 'unsplash', 'giphy'] as const).map((tab) => (
            <button
              key={tab}
              type="button"
              role="tab"
              aria-selected={sourceTab === tab}
              className={`media-library__source-tab ${sourceTab === tab ? 'media-library__source-tab--active' : ''}`}
              onClick={() => onSourceTabChange(tab)}
            >
              {tab === 'uploaded' ? 'Uploaded' : tab === 'unsplash' ? 'Unsplash' : 'Giphy'}
            </button>
          ))}
        </div>
      ) : null}

      {showSources ? <p className="hint media-library__source-hint">{sourceHint}</p> : null}

      <div className="media-library__body">
        <div className="media-library__grid" aria-label="Media placeholders">
          {['Slot A', 'Slot B', 'Slot C', 'Slot D'].map((label) => (
            <button key={label} type="button" className="media-library__tile">
              <span className="media-library__tile-label">{label}</span>
              <span className="hint">Drop or pick</span>
            </button>
          ))}
        </div>
        <aside className="media-library__sidebar glass-panel">
          <p className="eyebrow">Metadata</p>
          <p className="hint">Select a tile to edit title, alt text, and focal point.</p>
          <div className="media-library__integrations">
            <button
              type="button"
              className="button button--secondary"
              disabled={!integrations?.onRequestUnsplash}
              onClick={() => integrations?.onRequestUnsplash?.()}
            >
              <Icon name="plus" className="inline-icon" />
              <span>Unsplash</span>
            </button>
            <button
              type="button"
              className="button button--secondary"
              disabled={!integrations?.onRequestGiphy}
              onClick={() => integrations?.onRequestGiphy?.()}
            >
              <Icon name="plus" className="inline-icon" />
              <span>Giphy</span>
            </button>
          </div>
          <p className="hint" style={{ marginTop: '0.75rem' }}>
            {layout === 'page'
              ? 'External pickers stay disabled until the server exposes search endpoints and keys.'
              : 'Pass integration callbacks from the parent when Unsplash or Giphy pickers are configured.'}
          </p>
        </aside>
      </div>
    </div>
  )
}
