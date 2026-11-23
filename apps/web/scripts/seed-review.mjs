/**
 * Local review seed runner.
 *
 * Applies `review-seed.sql` to the throwaway Postgres the local review stack
 * runs on, so the admin screens have rows to render. An empty table cannot be
 * visually reviewed — there is no way to tell a correct table with no rows from
 * a broken table.
 *
 * Refuses to run against anything that is not a loopback host, for the same
 * reason the Playwright config does: this writes synthetic patient rows, and
 * there is no scenario in which that belongs on a shared database.
 *
 * Usage:
 *   node scripts/seed-review.mjs [containerName]
 */
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const CONTAINER = process.argv[2] ?? 'sigap-postgres';
const DB = process.argv[3] ?? 'sigap';
const USER = process.argv[4] ?? 'sigap';
const SQL = join(dirname(fileURLToPath(import.meta.url)), 'review-seed.sql');

/*
 * The guard. `docker exec` targets whatever container the name resolves to, and
 * a container name is not a host — so the check is on the container's own
 * published host, read back from Docker rather than assumed.
 */
const published = execFileSync(
	'docker',
	['inspect', '--format', '{{json .NetworkSettings.Ports}}', CONTAINER],
	{ encoding: 'utf8' }
);

const LOOPBACK = /127\.0\.0\.1|0\.0\.0\.0|\[::\]/;
if (!LOOPBACK.test(published)) {
	throw new Error(
		`Refusing to seed ${CONTAINER}: it publishes no loopback host (${published.trim()}). ` +
			'This seed writes synthetic patient rows and is for the local review stack only.'
	);
}

execFileSync('docker', ['cp', SQL, `${CONTAINER}:/tmp/review-seed.sql`], { stdio: 'inherit' });
execFileSync(
	'docker',
	['exec', CONTAINER, 'psql', '-U', USER, '-d', DB, '-v', 'ON_ERROR_STOP=1', '-f', '/tmp/review-seed.sql'],
	{ stdio: 'inherit' }
);

console.log(`seeded ${CONTAINER}/${DB} from review-seed.sql`);
