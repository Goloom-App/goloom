import { Icon } from '../../icons'

export type MediaIntegrationHandlers = {
  onRequestUnsplash?: () => void
  onRequestGiphy?: () => void
}

/**
 * Placeholder media workspace: grid slot + metadata sidebar.
 * Unsplash/Giphy are integration hooks for a future asset pipeline.
 */
export function MediaLibraryPanel({
  selectedLabel,
  integrations,
}: {
  selectedLabel: string
  integrations?: MediaIntegrationHandlers
}) {
  return (
    <div className="media-library">
      <div className="media-library__header">
        <p className="eyebrow">Media library</p>
        <p className="hint">Attach references for {selectedLabel}</p>
      </div>
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
            Pass integration callbacks from the parent when Unsplash or Giphy pickers are configured.
          </p>
        </aside>
      </div>
    </div>
  )
}
