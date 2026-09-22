import { useEffect, useRef } from 'react'

/**
 * Runs `callback` once `delay` has passed without `deps` changing.
 *
 * Used for draft autosave: saving on every keystroke would be one request per
 * character, and saving only on close would lose work if the tab went away.
 */
export function useDebouncedEffect(callback: () => void, deps: unknown[], delay: number) {
  const latest = useRef(callback)
  latest.current = callback

  useEffect(() => {
    const timer = window.setTimeout(() => latest.current(), delay)
    return () => window.clearTimeout(timer)
    // The caller owns the dependency list; that is the point of the hook.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, delay])
}
