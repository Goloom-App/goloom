import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { usePushNotifications } from '../../hooks/usePushNotifications'
import { SettingsCard } from './SettingsCard'

export function PushNotificationSettings() {
  const { t } = useTranslation()
  const {
    supported,
    loading,
    subscriptions,
    currentSub,
    prefs,
    permission,
    subscribe,
    subscribeError,
    setDeviceEnabled,
    subscribeDeviceEnabled,
    removeCurrentDevice,
    sendTest,
    testBusy,
    setTeamEnabled,
  } = usePushNotifications()
  const [busy, setBusy] = useState(false)
  const [testResult, setTestResult] = useState<'idle' | 'ok' | 'error'>('idle')

  if (!supported) {
    return (
      <SettingsCard title={t('pushNotifications.title')}>
        <p className="hint" data-testid="push-unsupported">
          {t('pushNotifications.unsupported')}
        </p>
      </SettingsCard>
    )
  }

  const hasCurrent = Boolean(currentSub)
  const currentEnabled = currentSub?.enabled === true
  const otherDevices = subscriptions.filter((sub) => sub !== currentSub)
  const denied = permission === 'denied'

  const permissionLabel =
    permission === 'granted'
      ? t('pushNotifications.permissionGranted')
      : permission === 'denied'
        ? t('pushNotifications.permissionDeniedLabel')
        : t('pushNotifications.permissionPrompt')

  async function run(action: () => Promise<unknown>) {
    setBusy(true)
    try {
      await action()
    } finally {
      setBusy(false)
    }
  }

  async function handleTest() {
    setTestResult('idle')
    try {
      await sendTest()
      setTestResult('ok')
    } catch {
      setTestResult('error')
    }
  }

  return (
    <SettingsCard title={t('pushNotifications.title')}>
      <p className="hint">{t('pushNotifications.hint')}</p>
      <p className="hint" data-testid="push-permission">
        {t('pushNotifications.permissionStatus', { state: permissionLabel })}
      </p>
      <div className="inline-cluster mt-1">
        {denied ? null : !hasCurrent ? (
          <button
            type="button"
            className="button button--secondary"
            onClick={() => void run(subscribe)}
            disabled={busy || loading}
            data-testid="push-enable"
          >
            {busy ? t('pushNotifications.enabling') : t('pushNotifications.enableButton')}
          </button>
        ) : currentEnabled ? (
          <>
            <button
              type="button"
              className="button button--primary"
              onClick={() => void handleTest()}
              disabled={testBusy}
              data-testid="push-test"
            >
              {t('pushNotifications.testButton')}
            </button>
            <button
              type="button"
              className="button button--secondary"
              onClick={() => void run(() => setDeviceEnabled(false))}
              disabled={busy}
              data-testid="push-disable"
            >
              {t('pushNotifications.disableButton')}
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              className="button button--secondary"
              onClick={() => void run(() => subscribeDeviceEnabled(true))}
              disabled={busy}
              data-testid="push-enable"
            >
              {t('pushNotifications.reEnableButton')}
            </button>
          </>
        )}
      </div>
      {denied ? (
        <p className="hint" data-testid="push-denied">
          {t('pushNotifications.permissionDenied')}
        </p>
      ) : null}
      {subscribeError ? (
        <p className="hint" data-testid="push-error">
          {permission === 'denied' ? t('pushNotifications.permissionDenied') : t('pushNotifications.subscribeError')}
        </p>
      ) : null}
      {testResult === 'ok' ? <p className="hint" data-testid="push-test-ok">{t('pushNotifications.testSuccess')}</p> : null}
      {testResult === 'error' ? <p className="hint" data-testid="push-test-error">{t('pushNotifications.testFailed')}</p> : null}

      {hasCurrent ? (
        <div className="mt-1">
          <button
            type="button"
            className="button button--secondary button--sm"
            onClick={() => void run(removeCurrentDevice)}
            disabled={busy}
            data-testid="push-remove"
          >
            {t('pushNotifications.removeDeviceThis')}
          </button>
        </div>
      ) : null}

      {subscriptions.length > 0 ? (
        <div className="mt-1">
          <p className="hint">{t('pushNotifications.devicesLabel')}</p>
          {otherDevices.length > 0 ? <p className="hint">{t('pushNotifications.otherDevicesHint')}</p> : null}
          <ul className="mt-1">
            {subscriptions.map((sub) => (
              <li key={sub.id} className="inline-cluster">
                <span>{sub.endpoint}</span>
                {sub === currentSub ? <span className="hint">({t('pushNotifications.thisDevice')})</span> : null}
                <span className="hint">{sub.enabled ? t('pushNotifications.deviceEnabled') : t('pushNotifications.deviceDisabled')}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {prefs.length > 0 ? (
        <div className="mt-1">
          <p className="hint">{t('pushNotifications.teamPrefsLabel')}</p>
          {prefs.map((pref) => (
            <label key={pref.team_id} className="field row-inline" data-testid={`push-team-pref-${pref.team_id}`}>
              <input
                type="checkbox"
                checked={pref.enabled}
                onChange={(event) => void setTeamEnabled({ teamID: pref.team_id, enabled: event.target.checked })}
              />
              <span className="ml-1">{t('pushNotifications.teamPrefName', { name: pref.team_name ?? pref.team_id })}</span>
            </label>
          ))}
        </div>
      ) : null}
    </SettingsCard>
  )
}