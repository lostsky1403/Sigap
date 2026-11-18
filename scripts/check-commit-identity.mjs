#!/usr/bin/env node
/**
 * check-commit-identity.mjs — CI gate for commit author/committer identity.
 *
 * `check-attribution.mjs` rejects AI-agent `Co-authored-by:` trailers in message
 * bodies. That is necessary but not sufficient: PR #96 was created with
 * `Author: sigap-ops <ops@local>` / `Committer: sigap-ops <ops@local>`, which no
 * trailer check sees. This script closes that gap for NEW commits only.
 *
 * It checks, for every commit in a range:
 *   1. AI-agent co-author trailers — by delegating to check-attribution.mjs, so
 *      the detection rules stay in one place;
 *   2. author and committer names/emails against generic, unlinked automation
 *      placeholders (e.g. sigap-ops <ops@local>, root@localhost, bot@local);
 *   3. in STRICT mode, that both author and committer are an authorized
 *      maintainer or an explicitly authorized automation identity.
 *
 * Scope: the strict rule is applied only on trusted maintainer/automation
 * branches. Fork pull requests are checked in permissive mode, so a genuine
 * external contributor whose email differs from the maintainer's is NOT blocked.
 * Historical commits are never scanned: a range of new commits is required.
 *
 * Usage:
 *   node scripts/check-commit-identity.mjs [--range A..B] [--mode auto|strict|permissive] [--branch name]
 *   node scripts/check-commit-identity.mjs --help
 *
 * Exit codes (same contract as check-attribution.mjs):
 *   0  clean
 *   1  a violation was found (the gate)
 *   2  missing prerequisite / cannot run (never a pass)
 *
 * Configuration (no secrets, no private emails):
 *   SIGAP_TRUSTED_BRANCHES       comma/space separated glob patterns of branches
 *                                where strict mode applies (auto mode only).
 *   SIGAP_AUTHORIZED_COMMITTERS  comma/semicolon separated emails or
 *                                `Name <email>` for authorized automation.
 *   SIGAP_ATTRIBUTION_ALLOWLIST  forwarded to check-attribution.mjs (human co-authors).
 */

import { execFileSync } from 'node:child_process';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

/** Resolve sibling scripts from this file, not from the caller's working directory. */
const MODULE_DIR = path.dirname(fileURLToPath(import.meta.url));

const DEFAULT_RANGE = 'HEAD~1..HEAD';

/** Verified GitHub-associated maintainer emails for lostsky1403 (already public in AGENTS.md). */
const MAINTAINER_EMAILS = [
	'moch22188ti@student.nurulfikri.ac.id',
	'114224549+lostsky1403@users.noreply.github.com',
	'lostsky1403@users.noreply.github.com',
];

/** Branches where strict mode applies unless overridden by SIGAP_TRUSTED_BRANCHES. */
const DEFAULT_TRUSTED_BRANCHES = [
	'main',
	'design/ui-ux-overhaul',
	'feat/sigap-selfhosted-auth',
	'release/*',
	'hotfix/*',
];

/**
 * Generic / unlinked automation placeholders that must never author or commit
 * on a trusted branch. Targeted at the observed failure shape; exact names are
 * used so a real person named Claude is not rejected.
 */
const PLACEHOLDER_NAME_PATTERNS = [
	/^sigap[-_ ]?ops$/i,
	/^internal-model$/i,
	/^(ci|ops|bot|automation|agent|unknown|root|noreply)$/i,
	/^traeagent$/i,
	/^codebuff$/i,
	/^claude( code)?$/i,
	/^anthropic$/i,
	/\[bot\]$/i,
];
const PLACEHOLDER_EMAIL_PATTERNS = [
	/@local$/i,
	/@localhost$/i,
	/\.local$/i,
	/^(ops|ci|bot|automation|agent|root|unknown|noreply|sigap-ops)([+-][^@]*)?@/i,
];

const USAGE = `Usage: node scripts/check-commit-identity.mjs [options]

Options:
  --range <A..B>          New commits to scan. Default: ${DEFAULT_RANGE}
  --mode auto|strict|permissive
                          auto (default): strict on trusted branches, permissive elsewhere.
  --branch <name>         Branch name to classify (auto mode). Defaults to
                          GITHUB_REF_NAME, then GITHUB_HEAD_REF, then current branch.
  -h, --help              Show this help.

Exit codes: 0 clean | 1 violation | 2 missing prerequisite.

Strict mode requires BOTH author and committer to be one of the maintainer
emails or an explicitly authorized automation identity (SIGAP_AUTHORIZED_COMMITTERS).
Permissive mode only rejects generic unlinked placeholders (and AI trailers),
so genuine external contributors are not blocked.
`;

function die(message, code = 2) {
	process.stderr.write(`FAIL: ${message}\n`);
	process.exit(code);
}

function git(args) {
	return execFileSync('git', args, {
		encoding: 'utf8',
		maxBuffer: 256 * 1024 * 1024,
		stdio: ['ignore', 'pipe', 'pipe'],
	});
}

function parseArgs(argv) {
	let range = null;
	let mode = 'auto';
	let branch = null;
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (arg === '-h' || arg === '--help') return { help: true };
		if (arg === '--range') {
			range = argv[++i];
			if (!range) die('--range needs a value (e.g. --range origin/main..HEAD)');
			continue;
		}
		if (arg.startsWith('--range=')) {
			range = arg.slice('--range='.length);
			if (!range) die('--range needs a value');
			continue;
		}
		if (arg === '--mode') {
			mode = argv[++i];
			if (!mode) die('--mode needs a value');
			continue;
		}
		if (arg.startsWith('--mode=')) mode = arg.slice('--mode='.length);
		else if (arg === '--branch') {
			branch = argv[++i];
			if (!branch) die('--branch needs a value');
		} else if (arg.startsWith('--branch=')) branch = arg.slice('--branch='.length);
		else die(`unknown argument: ${arg}\n\n${USAGE}`);
	}
	if (!['auto', 'strict', 'permissive'].includes(mode)) {
		die(`invalid --mode "${mode}" (expected auto, strict or permissive)`);
	}
	return { help: false, range, mode, branch };
}

function splitEnv(value) {
	return (value ?? '')
		.split(/[,;\s]+/)
		.map((entry) => entry.trim())
		.filter((entry) => entry !== '');
}

/** Extract the email from a `Name <email>` entry or a bare email. */
function emailOf(entry) {
	const match = /<([^<>]*)>/.exec(entry);
	return (match ? match[1] : entry).trim().toLowerCase();
}

function globMatch(branch, pattern) {
	const re = new RegExp('^' + pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*') + '$');
	return re.test(branch);
}

function isTrusted(branch) {
	const configured = splitEnv(process.env.SIGAP_TRUSTED_BRANCHES);
	const patterns = configured.length > 0 ? configured : DEFAULT_TRUSTED_BRANCHES;
	return patterns.some((pattern) => globMatch(branch, pattern));
}

function isPlaceholder(name, email, authorized) {
	if (authorized) return false;
	const n = (name ?? '').toLowerCase();
	const e = (email ?? '').toLowerCase();
	return PLACEHOLDER_NAME_PATTERNS.some((re) => re.test(n)) || PLACEHOLDER_EMAIL_PATTERNS.some((re) => re.test(e));
}

/** New commits as {sha, subject, authorName, authorEmail, committerName, committerEmail}. */
function readCommits(range) {
	const out = git(['log', '--format=%H%x00%an%x00%ae%x00%cn%x00%ce%x00%x1e', range]);
	const commits = [];
	for (const record of out.split('\x1e')) {
		const fields = record.split('\0').map((field) => field.trim());
		if (fields.length < 5 || fields[0] === '') continue;
		commits.push({
			sha: fields[0],
			authorName: fields[1],
			authorEmail: fields[2],
			committerName: fields[3],
			committerEmail: fields[4],
			subject: fields[0].slice(0, 10),
		});
	}
	return commits;
}

/** Run the existing trailer gate; return {code, output}. */
function runTrailerGate(root, range) {
	try {
		const output = execFileSync(
			process.execPath,
			[path.join(MODULE_DIR, 'check-attribution.mjs'), '--range', range],
			{ encoding: 'utf8', maxBuffer: 256 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'] },
		);
		return { code: 0, output };
	} catch (error) {
		return { code: Number(error?.status ?? 2), output: `${error?.stdout ?? ''}${error?.stderr ?? ''}` };
	}
}

function main() {
	const { help, range: requested, mode, branch: branchArg } = parseArgs(process.argv.slice(2));
	if (help) {
		process.stdout.write(USAGE);
		process.exit(0);
	}

	let root;
	try {
		if (git(['rev-parse', '--is-inside-work-tree']).trim() !== 'true') die('not inside a git work tree', 2);
		root = git(['rev-parse', '--show-toplevel']).trim();
	} catch (error) {
		die(`git unavailable: ${firstLine(error)}`, 2);
	}

	let range = requested ?? DEFAULT_RANGE;
	let commits;
	try {
		commits = readCommits(range);
	} catch (error) {
		if (requested === null) {
			try {
				range = 'HEAD^!';
				commits = readCommits(range);
			} catch (error2) {
				die(`cannot resolve a range of new commits: ${firstLine(error2)}`, 2);
			}
		} else {
			die(`cannot resolve range "${range}": ${firstLine(error)}`, 2);
		}
	}
	if (!range.includes('..') && !range.endsWith('^!')) {
		process.stderr.write(`WARN: range "${range}" is a bare revision; its whole ancestry is scanned.\n`);
	}

	const trailer = runTrailerGate(root, range);
	if (trailer.code === 2) die(`trailer gate could not run:\n${trailer.output}`, 2);

	let branch = branchArg ?? process.env.GITHUB_REF_NAME ?? process.env.GITHUB_HEAD_REF ?? '';
	if (branch === '' && mode === 'auto') {
		try {
			branch = git(['rev-parse', '--abbrev-ref', 'HEAD']).trim();
		} catch {
			branch = '';
		}
	}
	const effectiveMode = mode === 'auto' ? (isTrusted(branch) ? 'strict' : 'permissive') : mode;

	const authorized = new Set(
		splitEnv(process.env.SIGAP_AUTHORIZED_COMMITTERS).map(emailOf),
	);
	const allowed = new Set([...MAINTAINER_EMAILS, ...authorized]);

	const violations = [];
	for (const commit of commits) {
		const identities = [
			{ role: 'author', name: commit.authorName, email: commit.authorEmail },
			{ role: 'committer', name: commit.committerName, email: commit.committerEmail },
		];
		for (const identity of identities) {
			const email = (identity.email ?? '').toLowerCase();
			const okAuthorized = authorized.has(email);
			if (isPlaceholder(identity.name, identity.email, okAuthorized)) {
				violations.push({ sha: commit.sha, reason: `${identity.role} is an unlinked automation placeholder: ${identity.name} <${identity.email}>` });
				continue;
			}
			if (effectiveMode === 'strict' && !allowed.has(email)) {
				violations.push({ sha: commit.sha, reason: `${identity.role} email "${identity.email}" is not a maintainer or authorized automation identity` });
			}
		}
	}

	if (trailer.code === 1) {
		violations.push({ sha: '(trailer gate)', reason: 'AI-agent Co-authored-by trailer (from check-attribution.mjs)' });
	}

	if (violations.length === 0) {
		process.stdout.write(
			`PASS: ${commits.length} commit(s) have acceptable author/committer identity and no AI trailers ` +
				`(range ${range}, mode ${effectiveMode}, branch ${branch || 'unknown'})\n`,
		);
		if (trailer.output.trim() !== '') process.stdout.write(trailer.output);
		process.exit(0);
	}

	process.stdout.write(`FAIL: ${violations.length} identity/attribution violation(s) in range ${range} (mode ${effectiveMode})\n\n`);
	for (const violation of violations) {
		process.stdout.write(`  ${violation.sha.slice(0, 10)}  ${violation.reason}\n`);
	}
	if (trailer.output.trim() !== '') process.stdout.write(`\n${trailer.output}`);
	process.stdout.write(
		'\nSet the identity before committing: git config user.name/user.email, and check\n' +
			'`git var GIT_AUTHOR_IDENT`. Never use --author. Add authorized automation to\n' +
			'SIGAP_AUTHORIZED_COMMITTERS. See docs/operations/git-attribution.md.\n',
	);
	process.exit(1);
}

function firstLine(error) {
	const text = typeof error?.stderr === 'string' && error.stderr !== '' ? error.stderr : String(error?.message ?? error);
	return text.trim().split('\n')[0];
}

main();
