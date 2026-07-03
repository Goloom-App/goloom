import type { AppSection } from '../../types'

export interface TourStep {
  id: string
  /** data-tour anchor to spotlight; undefined renders a centered card. */
  target?: string
  titleKey: string
  textKey: string
  /**
   * How the step advances: the user clicks the spotlighted element, reaches
   * a section (covers multi-click paths like menus), or presses "Next".
   */
  advanceOn: 'click' | 'next' | 'section'
  /** Required for advanceOn === 'section'. */
  section?: AppSection
}

const WELCOME: TourStep = {
  id: 'welcome',
  titleKey: 'tour.welcomeTitle',
  textKey: 'tour.welcomeText',
  advanceOn: 'next',
}

const NAV_ACCOUNTS: TourStep = {
  id: 'nav-accounts',
  target: 'nav-accounts',
  titleKey: 'tour.navAccountsTitle',
  textKey: 'tour.navAccountsText',
  advanceOn: 'section',
  section: 'accounts',
}

const ACCOUNTS_CONNECT: TourStep = {
  id: 'accounts-connect',
  target: 'accounts-connect',
  titleKey: 'tour.accountsConnectTitle',
  textKey: 'tour.accountsConnectText',
  advanceOn: 'next',
}

const NEW_POST: TourStep = {
  id: 'new-post',
  target: 'new-post',
  titleKey: 'tour.newPostTitle',
  textKey: 'tour.newPostText',
  advanceOn: 'next',
}

/**
 * The guided tour walks through the real UI. Admins are routed through
 * platform setup first (register a provider, then connect an account);
 * regular users go straight to connecting an account.
 */
export function tourStepsForRole(isAdmin: boolean): TourStep[] {
  if (isAdmin) {
    return [
      WELCOME,
      {
        id: 'open-admin',
        target: 'user-menu',
        titleKey: 'tour.openAdminTitle',
        textKey: 'tour.openAdminText',
        advanceOn: 'section',
        section: 'admin',
      },
      {
        id: 'admin-providers',
        target: 'admin-tab-providers',
        titleKey: 'tour.adminProvidersTitle',
        textKey: 'tour.adminProvidersText',
        advanceOn: 'click',
      },
      {
        id: 'provider-form',
        target: 'admin-provider-form',
        titleKey: 'tour.providerFormTitle',
        textKey: 'tour.providerFormText',
        advanceOn: 'next',
      },
      NAV_ACCOUNTS,
      ACCOUNTS_CONNECT,
      NEW_POST,
      {
        id: 'finish',
        titleKey: 'tour.finishTitle',
        textKey: 'tour.finishAdminText',
        advanceOn: 'next',
      },
    ]
  }
  return [
    WELCOME,
    NAV_ACCOUNTS,
    ACCOUNTS_CONNECT,
    NEW_POST,
    {
      id: 'finish',
      titleKey: 'tour.finishTitle',
      textKey: 'tour.finishUserText',
      advanceOn: 'next',
    },
  ]
}
