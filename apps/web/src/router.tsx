import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'

import { sessionQueryOptions } from '@/features/auth/api'
import { SiteLayout } from '@/components/site/site-layout'
import { AppLayout } from '@/routes/app-layout'
import { DocsLayout, DocsPage } from '@/routes/docs'
import { HomePage } from '@/routes/home'
import { ForgotPasswordPage } from '@/routes/forgot-password'
import { LoginPage } from '@/routes/login'
import { MailboxPage } from '@/routes/mailbox'
import { RegisterPage } from '@/routes/register'
import { ResetPasswordPage } from '@/routes/reset-password'
import { SettingsAccountDetailPage } from '@/routes/settings/account-detail'
import { SettingsAccountsPage } from '@/routes/settings/accounts'
import { SettingsApiTokensPage } from '@/routes/settings/api-tokens'
import { SettingsLayout } from '@/routes/settings/settings-layout'
import { ThreadPage } from '@/routes/thread'
import { VerifyEmailPage } from '@/routes/verify-email'

interface RouterContext {
  queryClient: QueryClient
}

/**
 * The inbox is served by the generic `/$folder` route, so "go to the inbox" is
 * this link target rather than a literal "/inbox" path.
 */
export const INBOX = { to: '/$folder', params: { folder: 'inbox' } } as const

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
})

/** Reads the session once and caches it, so guards do not refetch per route. */
async function currentUser(queryClient: QueryClient) {
  return queryClient.ensureQueryData(sessionQueryOptions)
}

// ------------------------------------------------------------------ public --

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  // Returning an object without the key (rather than `key: undefined`) is what
  // makes the search param optional in the router's generated types.
  validateSearch: (search: Record<string, unknown>): { redirect?: string } =>
    typeof search.redirect === 'string' ? { redirect: search.redirect } : {},
  beforeLoad: async ({ context, search }) => {
    // Someone already signed in has no business on the sign-in page.
    if (await currentUser(context.queryClient)) {
      throw search.redirect ? redirect({ href: search.redirect }) : redirect(INBOX)
    }
  },
  component: LoginPage,
})

const registerRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/register',
  beforeLoad: async ({ context }) => {
    if (await currentUser(context.queryClient)) {
      throw redirect(INBOX)
    }
  },
  component: RegisterPage,
})

const forgotPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/forgot-password',
  component: ForgotPasswordPage,
})

const resetPasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/reset-password',
  validateSearch: (search: Record<string, unknown>): { token?: string } =>
    typeof search.token === 'string' ? { token: search.token } : {},
  component: ResetPasswordPage,
})

const verifyEmailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/verify-email',
  validateSearch: (search: Record<string, unknown>): { token?: string } =>
    typeof search.token === 'string' ? { token: search.token } : {},
  component: VerifyEmailPage,
})

// ----------------------------------------------------------------- public --
// The marketing home page and the documentation. Neither needs a session:
// someone evaluating x-mail, or integrating against it, should not have to
// create an account to read about it first.

const siteRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: '_site',
  component: SiteLayout,
})

const homeRoute = createRoute({
  getParentRoute: () => siteRoute,
  path: '/',
  beforeLoad: async ({ context }) => {
    // Someone already signed in wants their mail, not the pitch.
    if (await currentUser(context.queryClient)) {
      throw redirect(INBOX)
    }
  },
  component: HomePage,
})

const docsRoute = createRoute({
  getParentRoute: () => siteRoute,
  path: '/docs',
  component: DocsLayout,
})

const docsIndexRoute = createRoute({
  getParentRoute: () => docsRoute,
  path: '/',
  component: DocsPage,
})

const docsPageRoute = createRoute({
  getParentRoute: () => docsRoute,
  path: '/$page',
  component: DocsPage,
})

// --------------------------------------------------------------- protected --

const appLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: '_app',
  beforeLoad: async ({ context, location }) => {
    if (!(await currentUser(context.queryClient))) {
      // Remember where they were headed so sign-in can return them there.
      throw redirect({ to: '/login', search: { redirect: location.href } })
    }
  },
  component: AppLayout,
})

const folderRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/$folder',
  component: MailboxPage,
})

const threadRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/thread/$threadId',
  component: ThreadPage,
})

const settingsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/settings',
  component: SettingsLayout,
})

const settingsAccountsRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/accounts',
  component: SettingsAccountsPage,
})

const settingsAccountDetailRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/accounts/$accountId',
  component: SettingsAccountDetailPage,
})

const settingsApiTokensRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/api-tokens',
  component: SettingsApiTokensPage,
})

const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/settings/accounts' })
  },
})

const routeTree = rootRoute.addChildren([
  siteRoute.addChildren([homeRoute, docsRoute.addChildren([docsIndexRoute, docsPageRoute])]),
  loginRoute,
  registerRoute,
  forgotPasswordRoute,
  resetPasswordRoute,
  verifyEmailRoute,
  appLayoutRoute.addChildren([
    threadRoute,
    folderRoute,
    settingsRoute.addChildren([
      settingsIndexRoute,
      settingsAccountsRoute,
      settingsAccountDetailRoute,
      settingsApiTokensRoute,
    ]),
  ]),
])

export function buildRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: { queryClient },
    defaultPreload: 'intent',
  })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof buildRouter>
  }
}
