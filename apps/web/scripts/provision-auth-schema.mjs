// Provision the Better Auth `auth` schema using the project's official API.
// Mirrors the database block and plugins of src/lib/server/auth.better.ts.
// This is the CI/local equivalent of `npx auth migrate --config auth.better.ts`.
// Requires SIGAP_DATABASE_URL (the SIGAP Postgres connection string).
import { betterAuth } from 'better-auth';
import { jwt, twoFactor } from 'better-auth/plugins';
import { PostgresDialect } from 'kysely';
import pg from 'pg';
import { getMigrations } from 'better-auth/db/migration';

const url = process.env.SIGAP_DATABASE_URL;
if (!url) throw new Error('SIGAP_DATABASE_URL is required');
const pool = new pg.Pool({ connectionString: url });

const auth = betterAuth({
  secret: process.env.BETTER_AUTH_SECRET ?? 'sigap-ci-only-not-a-production-secret-0001',
  baseURL: process.env.BETTER_AUTH_URL ?? 'http://127.0.0.1:5173',
  database: { dialect: new PostgresDialect({ pool }), type: 'postgres', schemaName: 'auth' },
  emailAndPassword: { enabled: true, autoSignIn: false },
  plugins: [
    jwt({
      jwks: { keyPairConfig: { alg: 'ES256' }, rotationInterval: 60 * 60 * 24 * 30 },
      jwt: {
        issuer: process.env.SIGAP_AUTH_ISSUER ?? 'http://127.0.0.1:5173',
        audience: process.env.SIGAP_AUTH_AUDIENCE ?? 'authenticated',
        expirationTime: '1h',
      },
    }),
    twoFactor({
      issuer: 'SIGAP',
      totpOptions: { period: 30, digits: 6 },
      backupCodeOptions: { amount: 10, length: 10, storeBackupCodes: 'encrypted' },
      twoFactorCookieMaxAge: 10 * 60,
      trustDeviceMaxAge: 30 * 24 * 60 * 60,
      accountLockout: { enabled: true, maxFailedAttempts: 10, durationSeconds: 15 * 60 },
    }),
  ],
});

const { toBeCreated, toBeAdded, runMigrations } = await getMigrations(auth.options);
console.log('auth schema: toBeCreated=', (toBeCreated ?? []).length, 'toBeAdded=', (toBeAdded ?? []).length);
if ((toBeCreated?.length ?? 0) > 0 || (toBeAdded?.length ?? 0) > 0) await runMigrations();
await pool.end();
console.log('auth schema ready');
