import { useTranslation } from 'react-i18next'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { LogOut, Settings, ShieldCheck } from 'lucide-react'

import type { AppSection, UserRecord } from '../../types'

interface UserMenuProps {
  user: UserRecord | null
  collapsed?: boolean
  setSection: (section: AppSection) => void
  onSignOut: () => void
}

function userInitials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return '?'
  }
  if (parts.length === 1) {
    return parts[0].slice(0, 2).toUpperCase()
  }
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

export function UserMenu({ user, collapsed = false, setSection, onSignOut }: UserMenuProps) {
  const { t } = useTranslation()
  const name = user?.name?.trim() || user?.email || t('sidebar.userMenu')

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          className={`user-menu-trigger ${collapsed ? 'user-menu-trigger--collapsed' : ''}`}
          data-testid="user-menu-trigger"
          data-tour="user-menu"
          aria-label={t('sidebar.userMenu')}
          title={collapsed ? name : undefined}
        >
          <span className="user-menu-avatar" aria-hidden="true">
            {userInitials(name)}
          </span>
          {!collapsed && (
            <span className="user-menu-identity">
              <span className="user-menu-name">{name}</span>
              {user?.email && user.email !== name ? <span className="user-menu-email">{user.email}</span> : null}
            </span>
          )}
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="radix-dropdown-content" side="top" align="start" sideOffset={6}>
          <DropdownMenu.Item className="radix-dropdown-item" onSelect={() => setSection('settings')}>
            <Settings size={16} />
            {t('nav.settings')}
          </DropdownMenu.Item>
          {user?.globalRole === 'admin' && (
            <DropdownMenu.Item className="radix-dropdown-item" onSelect={() => setSection('admin')}>
              <ShieldCheck size={16} />
              {t('nav.admin')}
            </DropdownMenu.Item>
          )}
          <DropdownMenu.Separator className="divider" />
          <DropdownMenu.Item className="radix-dropdown-item radix-dropdown-item--danger" onSelect={onSignOut}>
            <LogOut size={16} />
            {t('sidebar.signOut')}
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
