import { useCallback, useEffect, useRef, useState } from 'react'

interface MessageBodyProps {
  html: string
  text: string
}

/** Fallback height used until the frame has been measured. */
const INITIAL_HEIGHT = 120
const MAX_HEIGHT = 5000

/**
 * Renders an email body.
 *
 * Email HTML is hostile by default and is never injected into the app's own
 * DOM. It goes into a sandboxed iframe whose sandbox grants only
 * `allow-same-origin` and `allow-popups`:
 *
 *   - no `allow-scripts`, so nothing in the message can execute — this is what
 *     actually neutralises the content;
 *   - `allow-same-origin` purely so the parent can measure the rendered height
 *     and size the frame to it. Granting it alongside `allow-scripts` would
 *     defeat the sandbox entirely, which is exactly why scripts stay off;
 *   - `allow-popups` so a link the reader clicks can open in a new tab, while
 *     top-level navigation of the app itself stays blocked.
 *
 * The server sanitises inbound HTML as well, so neither layer is load-bearing
 * on its own.
 */
export function MessageBody({ html, text }: MessageBodyProps) {
  const frameRef = useRef<HTMLIFrameElement>(null)
  const [height, setHeight] = useState(INITIAL_HEIGHT)

  const trimmed = html.trim()

  const measure = useCallback(() => {
    const body = frameRef.current?.contentDocument?.body
    if (!body) return
    const next = Math.min(Math.max(body.scrollHeight + 24, 48), MAX_HEIGHT)
    // Resizing the frame changes what the observer is watching, so a naive
    // update feeds straight back into another measurement. Ignoring sub-pixel
    // churn breaks that loop.
    setHeight((current) => (Math.abs(current - next) > 2 ? next : current))
  }, [])

  useEffect(() => {
    if (!trimmed) return
    const frame = frameRef.current
    if (!frame) return

    frame.addEventListener('load', measure)

    // Images and web fonts land after load, each changing the height. The
    // callback is deferred to the next frame so the observer is never still
    // running when the resize it caused is applied.
    const body = frame.contentDocument?.body
    let pending = 0
    const observer = body
      ? new ResizeObserver(() => {
          window.cancelAnimationFrame(pending)
          pending = window.requestAnimationFrame(measure)
        })
      : null
    if (body && observer) observer.observe(body)

    return () => {
      frame.removeEventListener('load', measure)
      observer?.disconnect()
      window.cancelAnimationFrame(pending)
    }
  }, [trimmed, measure])

  if (!trimmed) {
    return <pre className="whitespace-pre-wrap font-sans text-sm leading-relaxed">{text}</pre>
  }

  const document = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="referrer" content="no-referrer" />
    <base target="_blank" />
    <style>
      body { margin: 0; font: 14px/1.6 system-ui, -apple-system, sans-serif; color: #111; word-break: break-word; }
      img { max-width: 100%; height: auto; }
      a { color: #2563eb; }
      blockquote { margin: 0 0 0 .8rem; padding-left: .8rem; border-left: 2px solid #d4d4d8; color: #52525b; }
      pre { white-space: pre-wrap; }
    </style>
  </head>
  <body>${trimmed}</body>
</html>`

  return (
    <iframe
      ref={frameRef}
      title="Message content"
      sandbox="allow-same-origin allow-popups"
      referrerPolicy="no-referrer"
      srcDoc={document}
      onLoad={measure}
      style={{ height }}
      className="w-full border-0"
    />
  )
}
