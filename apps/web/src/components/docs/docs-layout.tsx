import { Link, Outlet, useParams } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { DOC_HOME, DOC_SECTIONS, docLink, docNeighbours, findDoc } from '@/docs/registry'
import { cn } from '@/lib/utils'

export function DocsLayout() {
  return (
    <div className="mx-auto flex w-full max-w-6xl gap-10 px-4 py-10">
      <DocsSidebar />
      <div className="min-w-0 flex-1">
        <Outlet />
      </div>
    </div>
  )
}

function DocsSidebar() {
  return (
    <nav
      aria-label="Documentation"
      className="sticky top-20 hidden h-fit w-56 shrink-0 md:block"
    >
      {DOC_SECTIONS.map((section) => (
        <div key={section.title} className="mb-6">
          <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {section.title}
          </p>
          <ul className="flex flex-col gap-0.5 border-l">
            {section.pages.map((page) => (
              <li key={page.slug}>
                <Link
                  {...docLink(page)}
                  className="-ml-px block border-l border-transparent py-1 pl-3 text-sm text-muted-foreground transition hover:border-border hover:text-foreground"
                  activeOptions={{ exact: true }}
                  activeProps={{
                    className: cn('border-foreground font-medium text-foreground'),
                  }}
                >
                  {page.title}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  )
}

/** Renders whichever page the URL names, with its heading and its neighbours. */
export function DocsPage() {
  const params = useParams({ strict: false }) as { page?: string }
  const slug = params.page ?? DOC_HOME.slug
  const page = findDoc(slug)

  if (!page) {
    return (
      <article className="max-w-2xl">
        <h1 className="text-2xl font-semibold tracking-tight">Page not found</h1>
        <p className="mt-4 text-muted-foreground">
          There is no documentation page called “{slug}”.
        </p>
        <Button asChild variant="outline" className="mt-6 gap-2">
          <Link to="/docs">
            <ArrowLeft className="size-4" />
            Back to the documentation
          </Link>
        </Button>
      </article>
    )
  }

  const { previous, next } = docNeighbours(page.slug)
  const Content = page.component

  return (
    <article className="max-w-2xl">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight text-balance">{page.title}</h1>
      </header>

      <Content />

      {previous || next ? (
        <nav className="mt-16 flex flex-wrap gap-3 border-t pt-6">
          {previous ? (
            <Button asChild variant="outline" className="h-auto justify-start gap-3 px-4 py-3">
              <Link {...docLink(previous)}>
                <ArrowLeft className="size-4 shrink-0" />
                <span className="flex flex-col items-start">
                  <span className="text-xs text-muted-foreground">Previous</span>
                  <span>{previous.title}</span>
                </span>
              </Link>
            </Button>
          ) : null}
          {next ? (
            <Button asChild variant="outline" className="ml-auto h-auto justify-end gap-3 px-4 py-3">
              <Link {...docLink(next)}>
                <span className="flex flex-col items-end">
                  <span className="text-xs text-muted-foreground">Next</span>
                  <span>{next.title}</span>
                </span>
                <ArrowRight className="size-4 shrink-0" />
              </Link>
            </Button>
          ) : null}
        </nav>
      ) : null}
    </article>
  )
}
