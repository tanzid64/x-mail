import { Link, Outlet } from '@tanstack/react-router'
import { Mail } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useSession } from '@/features/auth/api'
import { INBOX } from '@/router'

/**
 * The public shell: the home page and the documentation.
 *
 * It deliberately does not use the app's sidebar layout. Everything in here is
 * readable signed out, which is the point — someone evaluating x-mail, or
 * integrating against it, should not have to create an account first.
 */
export function SiteLayout() {
  return (
    <div className="flex min-h-svh flex-col bg-background">
      <SiteHeader />
      <div className="flex-1">
        <Outlet />
      </div>
      <SiteFooter />
    </div>
  )
}

export function SiteHeader() {
  // Signed out this is undefined, which is the normal case for these pages.
  const { data: user } = useSession()

  return (
    <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur">
      <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-4 px-4">
        <Link to="/" className="flex items-center gap-2 font-semibold">
          <Mail className="size-5" />
          x-mail
        </Link>

        <nav className="ml-4 flex items-center gap-1 text-sm">
          <Link
            to="/docs"
            className="rounded-md px-3 py-1.5 text-muted-foreground transition hover:bg-muted hover:text-foreground"
            activeProps={{ className: 'bg-muted text-foreground' }}
          >
            Documentation
          </Link>
        </nav>

        <div className="ml-auto flex items-center gap-2">
          {user ? (
            <Button asChild size="sm">
              <Link {...INBOX}>Go to inbox</Link>
            </Button>
          ) : (
            <>
              <Button asChild variant="ghost" size="sm">
                <Link to="/login">Sign in</Link>
              </Button>
              <Button asChild size="sm">
                <Link to="/register">Create account</Link>
              </Button>
            </>
          )}
        </div>
      </div>
    </header>
  )
}

export function SiteFooter() {
  return (
    <footer className="border-t">
      <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-6 text-sm text-muted-foreground">
        <span className="flex items-center gap-2 font-medium text-foreground">
          <Mail className="size-4" />
          x-mail
        </span>
        <Link to="/docs" className="hover:text-foreground">
          Documentation
        </Link>
        <Link to="/docs" className="hover:text-foreground">
          Installation
        </Link>
        <Link to="/docs" className="hover:text-foreground">
          API reference
        </Link>
        <span className="ml-auto">Multi-account mail you host yourself.</span>
      </div>
    </footer>
  )
}
