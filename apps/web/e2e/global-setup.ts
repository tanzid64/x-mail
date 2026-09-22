import { execFile } from 'node:child_process'
import { promisify } from 'node:util'

const run = promisify(execFile)

/**
 * Empties the end-to-end database before the suite runs.
 *
 * Without this the database accumulates across runs, and anything the schema
 * makes globally unique — a mail address, for instance — collides with a row
 * left over from last time, so a suite that passed once starts failing for
 * reasons that have nothing to do with the change under test.
 *
 * Truncating users cascades through accounts, domains, addresses, threads and
 * messages, so one statement clears everything.
 */
export default async function globalSetup() {
  await run(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      'postgres',
      'psql',
      '-U',
      'xmail',
      '-d',
      'xmail_e2e',
      '-c',
      'TRUNCATE users CASCADE',
    ],
    { cwd: '../..', timeout: 60_000 },
  ).catch((error: unknown) => {
    // A fresh database has no tables yet; the API creates them on startup.
    const message = error instanceof Error ? error.message : String(error)
    if (!message.includes('does not exist')) throw error
  })
}
