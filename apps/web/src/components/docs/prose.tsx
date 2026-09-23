import { useState, type ReactNode } from 'react'
import { Check, Copy, Info, TriangleAlert, ShieldAlert, Lightbulb } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * The building blocks every documentation page is written with.
 *
 * Content is authored as components rather than Markdown so that a code block
 * can carry a copy button and a callout can carry a tone, without adding a
 * Markdown pipeline and a syntax highlighter to the bundle.
 */

export function Lede({ children }: { children: ReactNode }) {
  return <p className="mt-4 text-lg text-muted-foreground text-pretty">{children}</p>
}

export function H2({ id, children }: { id: string; children: ReactNode }) {
  return (
    <h2 id={id} className="mt-12 scroll-mt-20 border-b pb-2 text-xl font-semibold tracking-tight">
      {children}
    </h2>
  )
}

export function H3({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <h3 id={id} className="mt-8 scroll-mt-20 font-semibold">
      {children}
    </h3>
  )
}

export function P({ children }: { children: ReactNode }) {
  return <p className="mt-4 leading-7 text-pretty">{children}</p>
}

export function UL({ children }: { children: ReactNode }) {
  return <ul className="mt-4 ml-6 list-disc space-y-2 leading-7">{children}</ul>
}

export function OL({ children }: { children: ReactNode }) {
  return <ol className="mt-4 ml-6 list-decimal space-y-2 leading-7">{children}</ol>
}

export function Code({ children }: { children: ReactNode }) {
  return (
    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[0.85em] break-words">
      {children}
    </code>
  )
}

/** A block of code or terminal output, with a copy button. */
export function CodeBlock({
  children,
  title,
  copyable = true,
}: {
  children: string
  title?: string
  copyable?: boolean
}) {
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(children)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard access can be refused; the text is selectable either way.
    }
  }

  return (
    <figure className="mt-4 overflow-hidden rounded-lg border">
      {title ? (
        <figcaption className="border-b bg-muted/50 px-4 py-2 font-mono text-xs text-muted-foreground">
          {title}
        </figcaption>
      ) : null}
      <div className="relative">
        <pre className="overflow-x-auto p-4 text-sm leading-relaxed">
          <code>{children}</code>
        </pre>
        {copyable ? (
          <button
            type="button"
            onClick={() => void copy()}
            aria-label="Copy to clipboard"
            className="absolute top-2 right-2 rounded-md border bg-background/80 p-1.5 text-muted-foreground opacity-0 backdrop-blur transition hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100 [figure:hover_&]:opacity-100"
          >
            {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
          </button>
        ) : null}
      </div>
    </figure>
  )
}

const CALLOUT_TONES = {
  note: { icon: Info, className: 'border-border bg-muted/50', iconClass: 'text-muted-foreground' },
  tip: {
    icon: Lightbulb,
    className: 'border-emerald-500/30 bg-emerald-500/5',
    iconClass: 'text-emerald-600 dark:text-emerald-400',
  },
  warning: {
    icon: TriangleAlert,
    className: 'border-amber-500/40 bg-amber-500/5',
    iconClass: 'text-amber-600 dark:text-amber-400',
  },
  security: {
    icon: ShieldAlert,
    className: 'border-destructive/30 bg-destructive/5',
    iconClass: 'text-destructive',
  },
} as const

export function Callout({
  tone = 'note',
  title,
  children,
}: {
  tone?: keyof typeof CALLOUT_TONES
  title?: string
  children: ReactNode
}) {
  const { icon: Icon, className, iconClass } = CALLOUT_TONES[tone]
  return (
    <aside className={cn('mt-6 flex gap-3 rounded-lg border p-4 text-sm', className)}>
      <Icon className={cn('mt-0.5 size-4 shrink-0', iconClass)} />
      <div className="min-w-0 [&>p:first-child]:mt-0">
        {title ? <p className="font-medium">{title}</p> : null}
        {children}
      </div>
    </aside>
  )
}

/** A simple reference table. Columns are fixed by the caller. */
export function Table({ head, children }: { head: string[]; children: ReactNode }) {
  return (
    <div className="mt-4 overflow-x-auto rounded-lg border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50">
          <tr>
            {head.map((label) => (
              <th key={label} className="px-4 py-2 text-left font-medium whitespace-nowrap">
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y">{children}</tbody>
      </table>
    </div>
  )
}

export function Row({ children }: { children: ReactNode }) {
  return <tr className="align-top">{children}</tr>
}

/** `children` is optional: a blank cell is a legitimate thing for a table to have. */
export function Cell({ children, mono }: { children?: ReactNode; mono?: boolean }) {
  return <td className={cn('px-4 py-2', mono && 'font-mono text-xs')}>{children}</td>
}

/** Numbered steps, for a procedure the reader follows in order. */
export function Steps({ children }: { children: ReactNode }) {
  return (
    <ol className="mt-6 space-y-6 border-l pl-6 [counter-reset:step]">{children}</ol>
  )
}

export function Step({ title, children }: { title: string; children: ReactNode }) {
  return (
    <li className="relative [counter-increment:step]">
      <span
        aria-hidden
        className="absolute -left-[2.1rem] flex size-6 items-center justify-center rounded-full border bg-background text-xs font-medium before:content-[counter(step)]"
      />
      <h3 className="font-medium">{title}</h3>
      <div className="[&>*:first-child]:mt-2">{children}</div>
    </li>
  )
}
