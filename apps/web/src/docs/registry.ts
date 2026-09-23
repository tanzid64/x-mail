import type { ComponentType } from 'react'

import { ConfigurationPage } from '@/docs/pages/configuration'
import { GettingStartedPage } from '@/docs/pages/getting-started'
import { InstallationPage } from '@/docs/pages/installation'
import { OverviewPage } from '@/docs/pages/overview'

export interface DocPage {
  /** URL slug. The overview is reached at /docs, everything else at /docs/<slug>. */
  slug: string
  title: string
  /** One line, shown under the title and in the section index. */
  description: string
  component: ComponentType
}

export interface DocSection {
  title: string
  pages: DocPage[]
}

/**
 * The documentation, in reading order.
 *
 * The sidebar, the previous/next links and the route all read this one list, so
 * a page cannot appear in the navigation without existing, or exist without
 * appearing.
 */
export const DOC_SECTIONS: DocSection[] = [
  {
    title: 'Introduction',
    pages: [
      {
        slug: 'overview',
        title: 'Overview',
        description: 'What x-mail is, and the one idea the rest of it follows from.',
        component: OverviewPage,
      },
    ],
  },
  {
    title: 'Installing',
    pages: [
      {
        slug: 'installation',
        title: 'Installation',
        description: 'Getting the stack running, with Docker or on your own machine.',
        component: InstallationPage,
      },
      {
        slug: 'configuration',
        title: 'Configuration',
        description: 'Every environment variable, what it does and which are required.',
        component: ConfigurationPage,
      },
    ],
  },
  {
    title: 'Using x-mail',
    pages: [
      {
        slug: 'getting-started',
        title: 'Getting started',
        description: 'From an empty inbox to a sent message, in four steps.',
        component: GettingStartedPage,
      },
    ],
  },
]

/** Every page, flattened into reading order. */
export const DOC_PAGES: DocPage[] = DOC_SECTIONS.flatMap((section) => section.pages)

/** The page reached at /docs with no slug. */
export const DOC_HOME = DOC_PAGES[0]!

export function findDoc(slug: string): DocPage | undefined {
  return DOC_PAGES.find((page) => page.slug === slug)
}

/** Where a page sits in the reading order, for the previous/next links. */
export function docNeighbours(slug: string): { previous?: DocPage; next?: DocPage } {
  const index = DOC_PAGES.findIndex((page) => page.slug === slug)
  if (index < 0) return {}
  return { previous: DOC_PAGES[index - 1], next: DOC_PAGES[index + 1] }
}

/** The canonical link props for a page, since the overview lives at /docs. */
export function docLink(page: DocPage) {
  return page.slug === DOC_HOME.slug
    ? ({ to: '/docs' } as const)
    : ({ to: '/docs/$page', params: { page: page.slug } } as const)
}
