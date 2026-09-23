import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { AtSign, Bell, KeyRound, Shield, SlidersHorizontal, Tag } from 'lucide-react'

import { cn } from '@/lib/utils'

/** Settings sections. Those not yet built are shown but not linkable. */
const SECTIONS = [
  { to: '/settings/accounts', label: 'Mail accounts', icon: AtSign, ready: true },
  { to: '/settings/api-tokens', label: 'API tokens', icon: KeyRound, ready: true },
  { to: '/settings/general', label: 'General', icon: SlidersHorizontal, ready: false },
  { to: '/settings/labels', label: 'Labels', icon: Tag, ready: false },
  { to: '/settings/notifications', label: 'Notifications', icon: Bell, ready: false },
  { to: '/settings/security', label: 'Security', icon: Shield, ready: false },
] as const

export function SettingsLayout() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">Manage your account and mail providers.</p>
      </div>

      <div className="flex flex-col gap-6 md:flex-row">
        <nav className="flex shrink-0 gap-1 overflow-x-auto md:w-52 md:flex-col">
          {SECTIONS.map((section) =>
            section.ready ? (
              <Link
                key={section.to}
                to={section.to}
                className={cn(
                  'flex items-center gap-2 rounded-md px-3 py-2 text-sm whitespace-nowrap transition',
                  pathname.startsWith(section.to)
                    ? 'bg-muted font-medium text-foreground'
                    : 'text-muted-foreground hover:bg-muted/60',
                )}
              >
                <section.icon className="size-4" />
                {section.label}
              </Link>
            ) : (
              <span
                key={section.to}
                aria-disabled
                title="Not built yet"
                className="flex cursor-not-allowed items-center gap-2 rounded-md px-3 py-2 text-sm whitespace-nowrap text-muted-foreground/50"
              >
                <section.icon className="size-4" />
                {section.label}
              </span>
            ),
          )}
        </nav>

        <div className="min-w-0 flex-1">
          <Outlet />
        </div>
      </div>
    </div>
  )
}
