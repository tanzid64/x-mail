import { Outlet } from '@tanstack/react-router'

import { AppHeader } from '@/components/layout/app-header'
import { AppSidebar } from '@/components/layout/app-sidebar'
import { VerifyEmailBanner } from '@/components/layout/verify-email-banner'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { useSession } from '@/features/auth/api'

export function AppLayout() {
  const { data: user } = useSession()

  // The route guard has already resolved the session, so this only happens if
  // the cache was cleared underneath us.
  if (!user) return null

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <AppHeader user={user} />
        <VerifyEmailBanner user={user} />
        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
