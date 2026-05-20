import { useTranslation } from 'react-i18next'
import { format, isValid, parseISO } from 'date-fns'

import type { UserRecord } from '../../types'

export function AdminUsersTab({ directoryUsers }: { directoryUsers: UserRecord[] }) {
  const { t } = useTranslation()
  const sorted = [...directoryUsers].sort((a, b) => a.name.localeCompare(b.name))
  const adminCount = sorted.filter((u) => u.globalRole === 'admin').length

  return (
    <div className="admin-tab-panel stack stack--lg">
      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div>
            <h2 className="admin-section__title">{t('admin.registeredUsers')}</h2>
            <p className="hint admin-section__hint">{t('admin.registeredUsersHint')}</p>
          </div>
          <div className="admin-head-stats">
            <span className="admin-pill admin-pill--muted">
              {t('admin.userCount', { count: sorted.length })}
            </span>
            <span className="admin-pill admin-pill--ok">
              {t('admin.adminCount', { count: adminCount })}
            </span>
          </div>
        </header>

        {sorted.length > 0 ? (
          <div className="admin-table-wrap">
            <table className="data-table admin-users-table">
              <thead>
                <tr>
                  <th>{t('common.name')}</th>
                  <th>{t('common.email')}</th>
                  <th>{t('common.access')}</th>
                  <th>{t('common.registered')}</th>
                </tr>
              </thead>
              <tbody>
                {sorted.map((user) => (
                  <tr key={user.id}>
                    <td>
                      <strong>{user.name}</strong>
                    </td>
                    <td className="admin-users-table__email">{user.email}</td>
                    <td>
                      <span
                        className={`admin-role-badge ${user.globalRole === 'admin' ? 'admin-role-badge--admin' : 'admin-role-badge--member'}`}
                      >
                        {user.globalRole === 'admin' ? t('roles.administrator') : t('roles.member')}
                      </span>
                    </td>
                    <td className="text-soft">
                      {user.createdAt && isValid(parseISO(user.createdAt)) ? format(parseISO(user.createdAt), 'PP') : t('common.emDash')}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="hint">{t('admin.noUsers')}</p>
        )}
      </section>
    </div>
  )
}
