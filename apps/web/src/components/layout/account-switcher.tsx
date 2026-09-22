import { Link } from '@tanstack/react-router'
import { Check, ChevronsUpDown, CircleAlert, Layers, Plus, Settings } from 'lucide-react'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@/components/ui/sidebar'
import { Skeleton } from '@/components/ui/skeleton'
import { useMailAccounts } from '@/features/accounts/api'
import { accountDotClass } from '@/features/accounts/types'
import { useUIStore } from '@/stores/ui'
import { cn } from '@/lib/utils'

export function AccountSwitcher() {
  const { data: accounts, isLoading } = useMailAccounts()
  const activeAccount = useUIStore((state) => state.activeAccount)
  const setActiveAccount = useUIStore((state) => state.setActiveAccount)

  if (isLoading) {
    return (
      <div className="px-2">
        <Skeleton className="h-12 w-full" />
      </div>
    )
  }

  const list = accounts ?? []
  const selected = list.find((account) => account.id === activeAccount)
  const showingAll = activeAccount === 'all' || !selected

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" aria-label="Switch mail account" className="border">
              {showingAll ? (
                <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-muted">
                  <Layers className="size-3.5" />
                </span>
              ) : (
                <span className={cn('size-2.5 shrink-0 rounded-full', accountDotClass(selected.color))} />
              )}

              <span className="flex min-w-0 flex-1 flex-col text-left leading-tight">
                <span className="truncate text-sm font-medium">
                  {showingAll ? 'All accounts' : selected.name}
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  {showingAll
                    ? list.length === 0
                      ? 'No accounts yet'
                      : `${list.length} account${list.length === 1 ? '' : 's'}`
                    : selected.api_key_hint}
                </span>
              </span>

              <ChevronsUpDown className="ml-auto size-4 shrink-0 opacity-50" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>

          <DropdownMenuContent align="start" className="w-64">
            <DropdownMenuLabel className="text-xs text-muted-foreground">Mail accounts</DropdownMenuLabel>

            {list.map((account) => (
              <DropdownMenuItem
                key={account.id}
                onSelect={() => setActiveAccount(account.id)}
                className="gap-2"
              >
                <span className={cn('size-2.5 shrink-0 rounded-full', accountDotClass(account.color))} />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate">{account.name}</span>
                  {account.status === 'invalid_credentials' ? (
                    <span className="flex items-center gap-1 text-xs text-destructive">
                      <CircleAlert className="size-3" />
                      Key rejected
                    </span>
                  ) : null}
                </span>
                {account.id === activeAccount ? <Check className="size-4 shrink-0" /> : null}
              </DropdownMenuItem>
            ))}

            {list.length > 0 ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem onSelect={() => setActiveAccount('all')} className="gap-2">
                  <Layers className="size-4" />
                  All accounts
                  {showingAll ? <Check className="ml-auto size-4" /> : null}
                </DropdownMenuItem>
              </>
            ) : null}

            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link to="/settings/accounts" className="gap-2">
                <Plus className="size-4" />
                Add mail account
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to="/settings/accounts" className="gap-2">
                <Settings className="size-4" />
                Manage accounts
              </Link>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
