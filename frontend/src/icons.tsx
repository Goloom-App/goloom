import type { ReactNode, SVGProps } from 'react'

export type IconName =
  | 'lock'
  | 'close'
  | 'sun'
  | 'moon'
  | 'calendar'
  | 'calendarGrid'
  | 'archive'
  | 'plus'
  | 'settings'
  | 'edit'
  | 'trash'
  | 'teams'
  | 'channels'
  | 'admin'
  | 'chevron-left'
  | 'chevron-right'

const iconPaths: Record<IconName, ReactNode> = {
  lock: (
    <>
      <rect x="5" y="11" width="14" height="10" rx="2" />
      <path d="M8 11V8a4 4 0 0 1 8 0v3" />
    </>
  ),
  close: (
    <>
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </>
  ),
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2.2M12 19.8V22M4.9 4.9l1.6 1.6M17.5 17.5l1.6 1.6M2 12h2.2M19.8 12H22M4.9 19.1l1.6-1.6M17.5 6.5l1.6-1.6" />
    </>
  ),
  moon: <path d="M20 14.2A8 8 0 0 1 9.8 4a9 9 0 1 0 10.2 10.2Z" />,
  calendar: (
    <>
      <rect x="3" y="5" width="18" height="16" rx="3" />
      <path d="M8 3v4M16 3v4M3 9h18" />
    </>
  ),
  calendarGrid: (
    <>
      <rect x="3" y="4" width="18" height="17" rx="2" />
      <path d="M3 9h18M9 4v17M15 4v17M3 14h18" />
    </>
  ),
  archive: (
    <>
      <path d="M4 7h16v11a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V7Z" />
      <path d="M3 7V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v2" />
      <path d="M9 12h6" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  settings: (
    <>
      <circle cx="12" cy="12" r="3.2" />
      <path d="M19.4 15a1 1 0 0 0 .2 1.1l.1.1a1.9 1.9 0 0 1 0 2.6l-.3.3a1.9 1.9 0 0 1-2.6 0l-.1-.1a1 1 0 0 0-1.1-.2 1 1 0 0 0-.6.9V20a2 2 0 0 1-2 2h-.5a2 2 0 0 1-2-2v-.2a1 1 0 0 0-.6-.9 1 1 0 0 0-1.1.2l-.1.1a1.9 1.9 0 0 1-2.6 0l-.3-.3a1.9 1.9 0 0 1 0-2.6l.1-.1a1 1 0 0 0 .2-1.1 1 1 0 0 0-.9-.6H4a2 2 0 0 1-2-2v-.5a2 2 0 0 1 2-2h.2a1 1 0 0 0 .9-.6 1 1 0 0 0-.2-1.1l-.1-.1a1.9 1.9 0 0 1 0-2.6l.3-.3a1.9 1.9 0 0 1 2.6 0l.1.1a1 1 0 0 0 1.1.2 1 1 0 0 0 .6-.9V4a2 2 0 0 1 2-2h.5a2 2 0 0 1 2 2v.2a1 1 0 0 0 .6.9 1 1 0 0 0 1.1-.2l.1-.1a1.9 1.9 0 0 1 2.6 0l.3.3a1.9 1.9 0 0 1 0 2.6l-.1.1a1 1 0 0 0-.2 1.1 1 1 0 0 0 .9.6h.2a2 2 0 0 1 2 2v.5a2 2 0 0 1-2 2h-.2a1 1 0 0 0-.9.6Z" />
    </>
  ),
  edit: (
    <>
      <path d="M4 20l4.2-1 9.4-9.4a2.1 2.1 0 0 0-3-3L5.2 16 4 20Z" />
      <path d="M13.5 7.5l3 3" />
    </>
  ),
  trash: (
    <>
      <path d="M4 7h16" />
      <path d="M9 7V4h6v3" />
      <path d="M7 7l1 13h8l1-13" />
      <path d="M10 11v5M14 11v5" />
    </>
  ),
  teams: (
    <>
      <path d="M16 21v-2a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v2" />
      <circle cx="9.5" cy="8" r="3" />
      <path d="M22 21v-2a4 4 0 0 0-3-3.9" />
      <path d="M16.5 4.1a3 3 0 0 1 0 5.8" />
    </>
  ),
  channels: (
    <>
      <path d="M10 13a5 5 0 0 0 7.5 4.4l2.2 1.3a1 1 0 0 0 1.5-.9V6.2a1 1 0 0 0-1.5-.9l-2.2 1.3A5 5 0 0 0 10 13Z" />
      <path d="M6 9H3v12h3" />
      <path d="M6 15H4" />
    </>
  ),
  admin: (
    <>
      <path d="M12 3 4 7v5c0 5 3.4 8 8 9 4.6-1 8-4 8-9V7l-8-4Z" />
      <path d="m9.5 12 1.7 1.7 3.8-3.8" />
    </>
  ),
  'chevron-left': <path d="m14.5 5-7 7 7 7" />,
  'chevron-right': <path d="m9.5 5 7 7-7 7" />,
}

export function Icon({
  name,
  className,
  ...props
}: SVGProps<SVGSVGElement> & { name: IconName }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className}
      {...props}
    >
      {iconPaths[name]}
    </svg>
  )
}
