// apps/web/src/lib/server/mail.ts
//
// Transactional-mail sink for the SIGAP self-hosted identity layer. Better Auth
// needs exactly two outbound messages — the email-verification link and the
// password-reset link — and this module is the ONLY place that "sends" them.
//
// WHAT THIS IS (dev path, and the only path that works today)
//
// The message is APPENDED to `<repo-root>/.local-preview/mail.log` and a
// one-line preview (recipient, subject, the actionable URL) is written to the
// server console. That file is the local mail capture: a developer opens it,
// copies the link and completes the flow. `.local-preview/` is gitignored at the
// repo root and already holds the dev service logs (`web.log`, `api.log`, …), so
// captured mail sits next to the process that produced it and never lands in a
// commit.
//
// The path is resolved by walking UP from `process.cwd()` to the nearest
// ancestor holding `pnpm-workspace.yaml` (or `.git`) — NOT by assuming the cwd.
// `scripts/dev/Run-V22Service.ps1` does `Set-Location apps\web` before serving
// the web app, so a naive `join(process.cwd(), '.local-preview')` would create a
// stray untracked directory inside `apps/web`. `SIGAP_MAIL_LOG` overrides the
// file path outright when a run needs mail somewhere explicit.
//
// WHAT THIS IS NOT (production)
//
// There is NO SMTP transport. `SIGAP_SMTP_URL` is the documented, env-driven
// seam where one belongs, and setting it today does NOT deliver anything:
// better-auth ships no SMTP client, this module adds no dependency, and the
// branch below says so out loud (one `console.error` per process) instead of
// pretending the message went out. When SMTP is configured, captured mail is
// deliberately NOT written to disk — a real deployment must not persist live
// reset links in a plaintext file — so the state is "loudly undelivered" rather
// than "silently leaked to a log".
//
// NEVER THROWS. Both callers are reached from public endpoints, and
// `/send-verification-email` in better-auth 1.7.20 re-throws a sender failure
// (dist/api/routes/email-verification.mjs: a real unverified address then gets
// 500 while an unknown one gets 200 — an enumeration oracle). A sink that
// cannot fail is what keeps those responses indistinguishable. See
// `design/email-recovery-mfa.md` §anti-enumeration.

import { appendFileSync, existsSync, mkdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

/** One outbound transactional message. `kind` is a log label, not a routing key. */
export interface SigapMail {
	to: string;
	subject: string;
	text: string;
	kind?: "email-verification" | "password-reset" | string;
}

const CAPTURE_FILE_NAME = "mail.log";
const MAX_ROOT_WALK = 12;

/** Cached resolution — the repo does not move under a running process. */
let cachedPath: string | null = null;

/**
 * Nearest ancestor of `start` that looks like the repository root, or `null`
 * when the walk hits the filesystem root first. Sync and memoized-in-effect:
 * it runs once per process.
 */
function findRepoRoot(start: string): string | null {
	let dir = resolve(start);
	for (let i = 0; i < MAX_ROOT_WALK; i++) {
		if (existsSync(join(dir, "pnpm-workspace.yaml")) || existsSync(join(dir, ".git"))) {
			return dir;
		}
		const parent = dirname(dir);
		if (parent === dir) break;
		dir = parent;
	}
	return null;
}

/**
 * Absolute path of the capture file: `SIGAP_MAIL_LOG` when set, else
 * `<repo-root>/.local-preview/mail.log`, else `<cwd>/.local-preview/mail.log`
 * as a last resort (a non-repository checkout).
 */
export function capturePath(): string {
	if (cachedPath) return cachedPath;
	const override = process.env.SIGAP_MAIL_LOG?.trim();
	if (override) {
		cachedPath = resolve(override);
		return cachedPath;
	}
	const root = findRepoRoot(process.cwd()) ?? process.cwd();
	cachedPath = join(root, ".local-preview", CAPTURE_FILE_NAME);
	return cachedPath;
}

/** True when an SMTP endpoint is configured. NOT a claim that mail is delivered. */
export function smtpConfigured(): boolean {
	return Boolean(process.env.SIGAP_SMTP_URL?.trim());
}

let smtpWarned = false;

/**
 * One line per process, on the error stream, because this is the state a
 * production deployment would sit in: a transport that is configured and
 * absent at the same time. Silence here would turn every password reset into an
 * invisible no-op.
 */
function warnSmtpNotImplemented(): void {
	if (smtpWarned) return;
	smtpWarned = true;
	console.error(
		"[sigap mail] SIGAP_SMTP_URL is set but NO SMTP transport is implemented in this build " +
			"(no mail dependency is installed). Transactional mail is NOT being delivered. " +
			"Unset SIGAP_SMTP_URL to use the local capture file. See design/email-recovery-mfa.md."
	);
}

/** First `http(s)://` token in the body — the link a developer actually needs. */
function firstUrl(text: string): string | null {
	const match = /https?:\/\/\S+/.exec(text);
	return match ? match[0] : null;
}

let captureDirReadyFor: string | null = null;

/**
 * Create the capture directory once per resolved path. Called inside the
 * non-throwing body, so a permission failure surfaces as a log line rather than
 * as a 500 on a public endpoint.
 */
function ensureCaptureDir(file: string): void {
	const dir = dirname(file);
	if (captureDirReadyFor === dir) return;
	mkdirSync(dir, { recursive: true });
	captureDirReadyFor = dir;
}

/**
 * Hand one message to the configured sink.
 *
 * - No `SIGAP_SMTP_URL`: append to the capture file, print the one-line preview.
 * - `SIGAP_SMTP_URL` set: **nothing is delivered** — one loud error explaining
 *   the missing transport, and no link written to disk.
 *
 * Resolves in both cases; never rejects. Callers in `auth.better.ts` pass the
 * `{ user, url, token }` payload Better Auth hands them straight through.
 */
export async function sendMail(mail: SigapMail): Promise<void> {
	const label = mail.kind ? `${mail.kind} ` : "";
	if (smtpConfigured()) {
		warnSmtpNotImplemented();
		console.error(`[sigap mail] DROPPED ${label}to=${mail.to} subject=${mail.subject}`);
		return;
	}

	const stamp = new Date().toISOString();
	let file = "";
	try {
		file = capturePath();
		ensureCaptureDir(file);
		appendFileSync(
			file,
			`=== ${stamp}${mail.kind ? ` | ${mail.kind}` : ""} | to: ${mail.to} | subject: ${mail.subject} ===\n${mail.text}\n\n`
		);
	} catch (err) {
		console.error(`[sigap mail] capture failed for ${label}to=${mail.to} (${file || "path unresolved"})`, err);
		return;
	}

	const url = firstUrl(mail.text);
	console.log(
		`[sigap mail] captured ${label}to=${mail.to} subject=${mail.subject}${url ? ` url=${url}` : ""} → ${file}`
	);
}
