import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MutationCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'

import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { buildRouter } from '@/router'
import { errorMessage, toast } from '@/lib/toast'
import './index.css'

const queryClient = new QueryClient({
  // A failed action must never look like nothing happened. Handling it here
  // rather than per-mutation means a new mutation is reported by default, and
  // forgetting to opt in is not a silent failure.
  mutationCache: new MutationCache({
    onError: (error, _variables, _context, mutation) => {
      if (mutation.meta?.silentError) return
      toast.error(errorMessage(error))
    },
  }),
  defaultOptions: {
    queries: {
      // Mail data is invalidated by websocket events rather than by polling,
      // so aggressive refetching would only add load.
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})

const router = buildRouter(queryClient)

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      {/* Radix tooltips need one provider above every trigger in the tree. */}
      <TooltipProvider delayDuration={300}>
        <RouterProvider router={router} />
      </TooltipProvider>
      <Toaster />
    </QueryClientProvider>
  </StrictMode>,
)
