export type PreviewLayout = 'card' | 'mastodon' | 'bluesky' | 'friendica'

export type SocialPreviewAttachment = {
  id: string
  previewUrl: string
  mimeType: string
  filename?: string
}

export type PreviewEngagement = {
  likes: number
  reposts: number
  replies: number
}
