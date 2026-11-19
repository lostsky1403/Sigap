#!/usr/bin/env node
/**
 * check-attribution.mjs — CI gate: fail when a NEW commit carries an AI-agent
 * `Co-authored-by:` trailer.
 *
 * The trailers this rejects were stamped by the agent tools (Trae / traeagent,
 * Codebuff, Claude Code, OMP's `internal-model`), never by git and never by the
 * committer's git config. `.githooks/commit-msg` strips them locally; this
 * script is the gate that does not depend on a developer having installed that
 * hook.
 *
 * It scans a CANVAS OF NEW COMMITS ONLY (`--range`), so the trailers already in
 * this repository's history can never turn CI red. A range is required to have
 * `A..B` shape; a bare revision scans its whole ancestry, which is warned about
 * for that reason.
 *
 * Usage:
 *   node scripts/check-attribution.mjs [--range <rev-range>]
 *   node scripts/check-attribution.mjs --help
 *
 * Default range: HEAD~1..HEAD (the commit just made). In a repository whose tip
 * is a root commit, the tip alone is scanned instead.
 *
 * Exit codes — the contract the CI job relies on:
 *   0  clean: no AI trailer in the scanned range
 *   1  an AI trailer was found (the gate)
 *   2  missing prerequisite: not a git work tree, git unusable, or the range
 *      cannot be resolved. Exit 2 is deliberately NOT a pass: CI installs the
 *      prerequisites and fetches full history, so a 2 there is a broken gate,
 *      and a gate that cannot run must not report success.
 *
 * Allowlist (the human-co-author rule)
 * A trailer is NEVER a violation when its email is allowlisted, whatever its
 * name looks like. Built-in human entries are listed in DEFAULT_HUMAN_ALLOWLIST;
 * add project-specific humans through the SIGAP_ATTRIBUTION_ALLOWLIST
 * environment variable (comma/space/semicolon separated), which the commit-msg
 * hook honours too. Any trailer that matches neither an AI email nor an AI name
 * is untouched by design — a human co-author is the normal case, not the
 * exception.
 */

import { execFileSync } from 'node:child_process';
import process from 'node:process';

const DEFAULT_RANGE = 'HEAD~1..HEAD';

/**
 * Known AI co-author emails (lowercased). Mirrored by `.githooks/commit-msg`;
 * these three cover the trailers actually present in this repository's history.
 */
const AI_EMAILS = new Map([
	['traeagent@users.noreply.github.com', 'Trae (traeagent)'],
	['noreply@codebuff.com', 'Codebuff'],
	['noreply@anthropic.com', 'Claude Code'],
]);

/**
 * Known AI co-author name tokens (lowercased). A name matches when it IS the
 * token or STARTS with it followed by a non-alphanumeric character, so
 * "Claude 4.7" and "Claude Opus 4.7" match while "Claudette" and a human
 * "Jean-Claude" do not. `internal-model` is the OMP agent's model name and
 * arrives with no email at all.
 */
const AI_NAMES = ['traeagent', 'codebuff', 'claude', 'internal-model', 'anthropic'];

/**
 * Human co-authors that must never be stripped. `real@example.com` is the
 * documented negative-test fixture (RFC 2606 reserved domain); the two others
 * are this repository's maintainer identities.
 */
const DEFAULT_HUMAN_ALLOWLIST = [
	'real@example.com',
	'114224549+lostsky1403@users.noreply.github.com',
	'moch22188ti@student.nurulfikri.ac.id',
];

const USAGE = `Usage: node scripts/check-attribution.mjs [--range <rev-range>]

Fail when a commit in <rev-range> carries an AI-agent Co-authored-by trailer.

  --range <rev-range>  Git revision range to scan, e.g. origin/main..HEAD or
                       <base-sha>..<head-sha>. Default: ${DEFAULT_RANGE}
  -h, --help           Show this help.

Exit codes: 0 clean | 1 AI trailer found | 2 missing prerequisite.

Detected as AI (case-insensitive):
  emails: ${[...AI_EMAILS.keys()].join(', ')}
  names:  ${AI_NAMES.join(', ')}  (matched as a whole word / name prefix)

Human co-authors are never touched. Emails in the built-in allowlist
(${DEFAULT_HUMAN_ALLOWLIST.join(', ')})
or in SIGAP_ATTRIBUTION_ALLOWLIST (comma/space/semicolon separated) always pass.
`;

function die(message, code = 2) {
	process.stderr.write(`FAIL: ${message}\n`);
	process.exit(code);
}

/** Run git and return stdout; throws with git's own message on failure. */
function git(args) {
	return execFileSync('git', args, {
		encoding: 'utf8',
		maxBuffer: 256 * 1024 * 1024,
		stdio: ['ignore', 'pipe', 'pipe'],
	});
}

function parseRangeArgs(argv) {
	let range = null;
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (arg === '-h' || arg === '--help') return { help: true };
		if (arg === '--range') {
			range = argv[++i];
			if (range === undefined) die('--range needs a value (e.g. --range origin/main..HEAD)');
			continue;
		}
		if (arg.startsWith('--range=')) {
			range = arg.slice('--range='.length);
			if (range === '') die('--range needs a value (e.g. --range origin/main..HEAD)');
			continue;
		}
		die(`unknown argument: ${arg}\n\n${USAGE}`);
	}
	return { help: false, range };
}

/**
 * A trailer line, decomposed. `email` is '' when the trailer has no <...> part
 * (`Co-Authored-By: internal-model` is exactly such a trailer in this history).
 */
function parseTrailer(line) {
	const match = /^[ \t]*co-authored-by:[ \t]*(\S.*)$/i.exec(line);
	if (!match) return null;

	const value = match[1];
	const emailMatch = /<([^<>]*)>/.exec(value);
	const email = emailMatch ? emailMatch[1].trim().toLowerCase() : '';
	const name = value.replace(/<[^<>]*>[\s\S]*$/, '').trim().toLowerCase();

	return { raw: line.trim(), email, name };
}

function aiNameMatch(name) {
	for (const token of AI_NAMES) {
		if (name === token) return token;
		if (!name.startsWith(token)) continue;
		const next = name.slice(token.length, token.length + 1);
		if (!/^[a-z0-9]/.test(next)) return token;
	}
	return null;
}

/** Returns a reason string when the trailer is AI attribution, else null. */
function aiReason(trailer) {
	if (trailer.email !== '' && AI_EMAILS.has(trailer.email)) return AI_EMAILS.get(trailer.email);
	const name = aiNameMatch(trailer.name);
	if (name !== null) return name === 'internal-model' ? 'internal-model (OMP)' : `AI co-author name "${name}"`;
	return null;
}

function buildAllowlist() {
	const allow = new Set(DEFAULT_HUMAN_ALLOWLIST.map((email) => email.toLowerCase()));
	const extra = process.env.SIGAP_ATTRIBUTION_ALLOWLIST ?? '';
	for (const entry of extra.split(/[,;\s]+/)) {
		const email = entry.trim().replace(/^<|>$/g, '').toLowerCase();
		if (email !== '') allow.add(email);
	}
	return allow;
}

/** All commits in the range as {sha, subject, lines}. Empty range -> []. */
function readCommits(range) {
	// NUL fields: a commit message cannot contain NUL, so %B stays parseable
	// however the body is formatted.
	const out = git(['log', '--format=%H%x00%B%x00', range]);
	const fields = out.split('\0');
	const commits = [];
	for (let i = 0; i + 1 < fields.length; i += 2) {
		const sha = fields[i].trim();
		if (sha === '') continue;
		const body = fields[i + 1];
		const lines = body.split('\n');
		commits.push({ sha, subject: (lines[0] ?? '').trim(), lines });
	}
	return commits;
}

function isGitWorkTree() {
	try {
		return git(['rev-parse', '--is-inside-work-tree']).trim() === 'true';
	} catch {
		return false;
	}
}

function main() {
	const { help, range: requested } = parseRangeArgs(process.argv.slice(2));
	if (help) {
		process.stdout.write(USAGE);
		process.exit(0);
	}

	if (!isGitWorkTree()) {
		die('not inside a git work tree; run this from the repository checkout', 2);
	}

	let range = requested ?? DEFAULT_RANGE;
	let commits;
	try {
		commits = readCommits(range);
	} catch (error) {
		if (requested !== null) {
			die(`cannot resolve range "${range}": ${firstLine(error)}\n      A shallow checkout cannot see the base commit; fetch the history (or use fetch-depth: 0).`, 2);
		}
		// Default range, e.g. a one-commit repository with no HEAD~1: scan the tip.
		try {
			range = 'HEAD^!';
			commits = readCommits(range);
		} catch (error2) {
			die(`cannot resolve range "${DEFAULT_RANGE}" nor "HEAD^!": ${firstLine(error2)}`, 2);
		}
	}

	if (!range.includes('..') && !range.endsWith('^!')) {
		process.stderr.write(
			`WARN: range "${range}" is a bare revision, so its whole ancestry is scanned.\n` +
				'      Pass A..B to scan only new commits.\n',
		);
	}

	const allow = buildAllowlist();
	const violations = [];

	for (const commit of commits) {
		for (const line of commit.lines) {
			const trailer = parseTrailer(line);
			if (trailer === null) continue;
			if (trailer.email !== '' && allow.has(trailer.email)) continue;
			const reason = aiReason(trailer);
			if (reason === null) continue;
			violations.push({ sha: commit.sha, subject: commit.subject, trailer, reason });
		}
	}

	if (violations.length === 0) {
		process.stdout.write(`PASS: no AI attribution trailers in ${commits.length} commit(s) (range ${range})\n`);
		process.exit(0);
	}

	process.stdout.write(`FAIL: ${violations.length} AI attribution trailer(s) in range ${range}\n\n`);
	for (const violation of violations) {
		process.stdout.write(`  ${violation.sha.slice(0, 10)}  ${violation.subject}\n`);
		process.stdout.write(`    ${violation.trailer.raw}   <- ${violation.reason}\n`);
	}
	process.stdout.write(
		'\nAgent tools (Trae, Codebuff, Claude Code, OMP) append these when they\n' +
			'create a commit. Remove the line and re-commit:\n' +
			'  git commit --amend            # for the tip\n' +
			'  git rebase --exec ... / git filter-repo   # for older commits in the range\n' +
			'\nPrevent it locally:  git config core.hooksPath .githooks\n' +
			'If the address is a human co-author, add it to SIGAP_ATTRIBUTION_ALLOWLIST.\n' +
			'See docs/operations/git-attribution.md.\n',
	);
	process.exit(1);
}

function firstLine(error) {
	const text = typeof error?.stderr === 'string' && error.stderr !== '' ? error.stderr : String(error?.message ?? error);
	return text.trim().split('\n')[0];
}

main();
