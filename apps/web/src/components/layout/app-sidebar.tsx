import { Link, useRouterState } from '@tanstack/react-router'
import { PenSquare, Mail, Settings } from 'lucide-react'

import { AccountSwitcher } from '@/components/layout/account-switcher'

import { Button } from '@/components/ui/button'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarMenuBadge,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { useFolderCounts } from '@/features/mailbox/api'
import { FOLDERS } from '@/lib/folders'
import { useUIStore } from '@/stores/ui'

/** Sidebar folders read their badge from the system label the API counts by. */
const FOLDER_COUNT_KEYS: Record<string, string> = {
  inbox: 'INBOX',
  sent: 'SENT',
  drafts: 'DRAFT',
  archive: 'ARCHIVE',
  spam: 'SPAM',
  trash: 'TRASH',
}

export function AppSidebar() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const account = useUIStore((state) => state.activeAccount)
  const { data: counts } = useFolderCounts(account)

  return (
    <Sidebar>
      <SidebarHeader className="gap-3">
        <div className="flex items-center gap-2 px-2 py-1 font-medium">
          <span className="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Mail className="size-4" />
          </span>
          x-mail
        </div>

        <AccountSwitcher />

        {/* Compose is wired up in the sending phase. */}
        <Button className="justify-start gap-2" disabled>
          <PenSquare className="size-4" />
          Compose
        </Button>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Mailbox</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {FOLDERS.map((folder) => {
                const unread = counts?.[FOLDER_COUNT_KEYS[folder.slug] ?? ''] ?? 0
                return (
                  <SidebarMenuItem key={folder.slug}>
                    <SidebarMenuButton asChild isActive={pathname === `/${folder.slug}`}>
                      <Link to="/$folder" params={{ folder: folder.slug }}>
                        <folder.icon className="size-4" />
                        <span>{folder.label}</span>
                      </Link>
                    </SidebarMenuButton>
                    {unread > 0 ? <SidebarMenuBadge>{unread}</SidebarMenuBadge> : null}
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild isActive={pathname.startsWith('/settings')}>
              <Link to="/settings/accounts">
                <Settings className="size-4" />
                <span>Settings</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}
