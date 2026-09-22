/**
 * A stand-in for the Resend API, used by the end-to-end suite.
 *
 * The real service cannot be part of an automated test: it needs live
 * credentials and would send real mail. This implements just enough of the
 * surface for the account-connection flow — and, importantly, rejects an
 * unknown key, so the "bad API key" path is exercised for real rather than
 * mocked out in the browser.
 */
import { createServer } from 'node:http'

const PORT = Number(process.env.STUB_RESEND_PORT ?? 8099)

// Any key with this prefix is accepted; everything else gets a 401.
const VALID_KEY_PREFIX = 're_e2e_valid'

const DOMAINS = [
  {
    id: 'prov_dom_verified',
    name: 'company.test',
    status: 'verified',
    region: 'us-east-1',
    created_at: new Date().toISOString(),
    records: [],
  },
  {
    id: 'prov_dom_pending',
    name: 'pending.test',
    status: 'not_started',
    region: 'us-east-1',
    created_at: new Date().toISOString(),
    records: [
      {
        record: 'SPF',
        name: 'send',
        type: 'TXT',
        ttl: 'Auto',
        status: 'not_started',
        value: 'v=spf1 include:amazonses.com ~all',
      },
      {
        record: 'DKIM',
        name: 'resend._domainkey',
        type: 'TXT',
        ttl: 'Auto',
        status: 'not_started',
        value: 'p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ',
      },
    ],
  },
]

function send(res, status, payload) {
  const body = JSON.stringify(payload)
  res.writeHead(status, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) })
  res.end(body)
}

const server = createServer((req, res) => {
  const token = (req.headers.authorization ?? '').replace(/^Bearer\s+/i, '')
  if (!token.startsWith(VALID_KEY_PREFIX)) {
    send(res, 401, { statusCode: 401, name: 'validation_error', message: 'API key is invalid' })
    return
  }

  const url = new URL(req.url, `http://localhost:${PORT}`)

  if (req.method === 'GET' && url.pathname === '/domains') {
    send(res, 200, { data: DOMAINS })
    return
  }

  if (req.method === 'GET' && url.pathname.startsWith('/domains/')) {
    const id = url.pathname.slice('/domains/'.length)
    const domain = DOMAINS.find((d) => d.id === id)
    if (!domain) {
      send(res, 404, { statusCode: 404, name: 'not_found', message: 'Domain not found' })
      return
    }
    send(res, 200, domain)
    return
  }

  if (req.method === 'POST' && url.pathname === '/emails') {
    send(res, 200, { id: `stub_${Date.now()}` })
    return
  }

  send(res, 404, { statusCode: 404, name: 'not_found', message: 'Not found' })
})

server.listen(PORT, () => {
  console.log(`stub resend listening on ${PORT}`)
})
