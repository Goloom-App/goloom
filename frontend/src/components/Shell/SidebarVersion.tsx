import { useTranslation } from 'react-i18next'
import { ArrowUpCircle } from 'lucide-react'

import type { BackendVersionInfo } from '../../api'

const RELEASES_URL = 'https://github.com/Goloom-App/goloom/releases'

interface SidebarVersionProps {
  info: BackendVersionInfo | null
  collapsed?: boolean
}

// SidebarVersion shows the running goloom version in the sidebar footer and,
// when the server's update check found a newer release, turns into a link to
// the release notes. Renders nothing until the version is known.
export function SidebarVersion({ info, collapsed = false }: SidebarVersionProps) {
  const { t } = useTranslation()
  if (!info?.current) {
    return null
  }

  const label = t('sidebar.version', { version: info.current })

  if (info.update_available && info.latest) {
    const href = `${RELEASES_URL}/tag/${encodeURIComponent(info.latest)}`
    const updateTitle = t('sidebar.updateTo', { version: info.latest })
    return (
      <a
        className="sidebar-version sidebar-version--update"
        href={href}
        target="_blank"
        rel="noreferrer noopener"
        data-testid="sidebar-version"
        title={collapsed ? updateTitle : `${label} — ${updateTitle}`}
        aria-label={updateTitle}
      >
        <ArrowUpCircle size={14} className="sidebar-version__icon" aria-hidden="true" />
        {!collapsed && (
          <span className="sidebar-version__text">
            <span className="sidebar-version__current">{label}</span>
            <span className="sidebar-version__badge">{t('sidebar.updateAvailable')}</span>
          </span>
        )}
      </a>
    )
  }

  return (
    <div
      className="sidebar-version"
      data-testid="sidebar-version"
      title={collapsed ? label : t('sidebar.upToDate')}
    >
      {collapsed ? info.current : label}
    </div>
  )
}
