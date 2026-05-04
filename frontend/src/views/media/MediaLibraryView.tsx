import { useState } from 'react'
import { MediaLibraryPanel, type MediaSourceTab } from '../../components/Media/MediaLibraryPanel'

export function MediaLibraryView({
  teamName,
  onRequestUnsplash,
  onRequestGiphy,
}: {
  teamName: string
  onRequestUnsplash?: () => void
  onRequestGiphy?: () => void
}) {
  const [sourceTab, setSourceTab] = useState<MediaSourceTab>('uploaded')

  return (
    <div className="media-library-view glass-panel">
      <MediaLibraryPanel
        selectedLabel={teamName}
        layout="page"
        sourceTab={sourceTab}
        onSourceTabChange={setSourceTab}
        integrations={{
          onRequestUnsplash,
          onRequestGiphy,
        }}
      />
    </div>
  )
}
