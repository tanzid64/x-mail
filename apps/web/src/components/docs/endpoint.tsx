import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

/**
 * One endpoint, laid out the same way every time.
 *
 * Consistency is the whole value of a reference: a reader who has understood
 * one entry should not have to work out how to read the next.
 */

const METHOD_COLOURS: Record<string, string> = {
  GET: 'bg-sky-500/10 text-sky-700 dark:text-sky-400 border-sky-500/30',
  POST: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-emerald-500/30',
  PATCH: 'bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/30',
  PUT: 'bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-500/30',
  DELETE: 'bg-destructive/10 text-destructive border-destructive/30',
}

export function Endpoint({
  method,
  path,
  id,
  children,
}: {
  method: keyof typeof METHOD_COLOURS | string
  path: string
  id: string
  children: ReactNode
}) {
  return (
    <section id={id} className="mt-10 scroll-mt-20 rounded-lg border">
      <header className="flex flex-wrap items-center gap-3 border-b bg-muted/30 px-4 py-3">
        <span
          className={cn(
            'rounded-md border px-2 py-0.5 font-mono text-xs font-semibold',
            METHOD_COLOURS[method] ?? 'bg-muted text-muted-foreground',
          )}
        >
          {method}
        </span>
        <code className="font-mono text-sm break-all">{path}</code>
      </header>
      <div className="px-4 pb-4 [&>*:first-child]:mt-3">{children}</div>
    </section>
  )
}

/** A labelled block inside an endpoint — parameters, body, response. */
export function Part({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="mt-5">
      <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{title}</p>
      <div className="[&>*:first-child]:mt-2">{children}</div>
    </div>
  )
}

/** One field of a request body or query string. */
export function Field({
  name,
  type,
  required,
  children,
}: {
  name: string
  type: string
  required?: boolean
  children: ReactNode
}) {
  return (
    <div className="border-b py-2.5 last:border-b-0">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <code className="font-mono text-sm font-medium">{name}</code>
        <span className="font-mono text-xs text-muted-foreground">{type}</span>
        {required ? (
          <span className="text-xs font-medium text-destructive">required</span>
        ) : (
          <span className="text-xs text-muted-foreground">optional</span>
        )}
      </div>
      <div className="mt-1 text-sm text-muted-foreground [&>*:first-child]:mt-0">{children}</div>
    </div>
  )
}

export function Fields({ children }: { children: ReactNode }) {
  return <div className="mt-2 rounded-md border px-4">{children}</div>
}
