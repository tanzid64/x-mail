/**
 * Thin fetch wrapper around the Go API.
 *
 * Requests always carry the session cookie. In development Vite proxies /api to
 * the API server, so this is same-origin and the cookie behaves exactly as it
 * will in production behind Nginx.
 */

const API_BASE = '/api/v1'

/** The error envelope the Go API returns: `{ error: { code, message, fields } }`. */
export interface ApiErrorBody {
  code: string
  message: string
  fields?: Record<string, string>
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  /** Per-field validation messages, keyed by the request field name. */
  readonly fields: Record<string, string>

  constructor(status: number, body: ApiErrorBody) {
    super(body.message)
    this.name = 'ApiError'
    this.status = status
    this.code = body.code
    this.fields = body.fields ?? {}
  }

  /** True when the session is missing or expired. */
  get isUnauthorized(): boolean {
    return this.status === 401
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
}

export async function apiFetch<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, signal } = options

  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      signal,
      // Without this the browser omits the session cookie on cross-origin setups.
      credentials: 'include',
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    // fetch only rejects for network-level failures, which are worth
    // distinguishing from an API that answered with an error.
    throw new ApiError(0, {
      code: 'network_error',
      message: 'Could not reach the server. Check your connection and try again.',
    })
  }

  if (response.status === 204) {
    return undefined as T
  }

  const text = await response.text()
  const payload = text ? safeParse(text) : undefined

  if (!response.ok) {
    const body = (payload as { error?: ApiErrorBody } | undefined)?.error
    throw new ApiError(
      response.status,
      body ?? { code: 'unexpected_error', message: `Request failed with status ${response.status}.` },
    )
  }

  return payload as T
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}
