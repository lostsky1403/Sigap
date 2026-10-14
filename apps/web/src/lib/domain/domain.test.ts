import { describe, expect, it } from 'vitest';
import {
	UNRESOLVED,
	facilityName,
	indexBy,
	isUnresolved,
	practitionerName,
	resolveMany,
	resolveName
} from './joins';
import {
	EMPTY,
	formatDate,
	formatDateTime,
	formatNumber,
	formatRelativeTime,
	formatTime,
	isStale,
	sentenceCase,
	truncate
} from './format';

/* ------------------------------------------------------------------ *
 * joins
 * ------------------------------------------------------------------ */

const FACILITIES = [
	{ id: 'f1', name: 'RS Pusat Kota' },
	{ id: 'f2', name: 'Puskesmas Timur' }
];

describe('joins', () => {
	it('resolves a known id to its human label', () => {
		expect(facilityName(new Map(FACILITIES.map((f) => [f.id, f])), 'f1')).toBe('RS Pusat Kota');
	});

	it('renders a missing id as the placeholder', () => {
		expect(facilityName(new Map(), 'f1')).toBe(UNRESOLVED);
	});

	it('renders a null id as the placeholder', () => {
		expect(facilityName(new Map(FACILITIES.map((f) => [f.id, f])), null)).toBe(UNRESOLVED);
		expect(facilityName(new Map(FACILITIES.map((f) => [f.id, f])), undefined)).toBe(UNRESOLVED);
	});

	it('renders an out-of-scope reference as the placeholder', () => {
		// An id that exists but was not returned to this viewer is simply absent
		// from the table. Showing anything else would leak that it exists.
		const visible = new Map(FACILITIES.map((f) => [f.id, f]));
		expect(facilityName(visible, 'f-other-facility')).toBe(UNRESOLVED);
		expect(facilityName(visible, 'f-out-of-scope')).toBe(UNRESOLVED);
	});

	it('never exposes a raw id as a display name', () => {
		const label = facilityName(new Map(), '0f8fad5b-d9cb-469f-a165-70867728950e');
		expect(label).toBe(UNRESOLVED);
		expect(label).not.toContain('0f8fad5b');
	});

	it('does not fabricate a name from a familiar-looking id', () => {
		const table = new Map([['rumah-sakit-pusat', { id: 'rumah-sakit-pusat', name: 'A' }]]);
		expect(facilityName(table, 'puskesmas-utara')).toBe(UNRESOLVED);
	});

	it('treats an empty or whitespace label as unresolved', () => {
		const table = new Map([
			['a', { name: '' }],
			['b', { name: '   ' }]
		]);
		expect(facilityName(table, 'a')).toBe(UNRESOLVED);
		expect(facilityName(table, 'b')).toBe(UNRESOLVED);
	});

	it('works with a plain object table', () => {
		expect(facilityName({ f1: { name: 'RS Pusat Kota' } }, 'f1')).toBe('RS Pusat Kota');
	});

	it('resolves simple string tables', () => {
		expect(resolveName({ f1: 'Pusat Kota' }, 'f1')).toBe('Pusat Kota');
		expect(resolveName({ f1: 'Pusat Kota' }, 'f2')).toBe(UNRESOLVED);
	});

	it('falls back to name when full_name is absent', () => {
		const table = new Map([
			['d1', { full_name: 'dr. Anita' }],
			['d2', { name: 'dr. Budi' }],
			['d3', {}]
		]);
		expect(practitionerName(table, 'd1')).toBe('dr. Anita');
		expect(practitionerName(table, 'd2')).toBe('dr. Budi');
		expect(practitionerName(table, 'd3')).toBe(UNRESOLVED);
	});

	it('indexes rows and skips those without a usable id', () => {
		const table = indexBy(
			[
				{ id: 'a', name: 'A' },
				{ id: '', name: 'Empty' },
				{ id: null, name: 'Null' }
			],
			(row) => row.id
		);
		expect(table.size).toBe(1);
		expect(resolveName(table as never, 'a')).toBe(UNRESOLVED);
		expect(facilityName(table, 'a')).toBe('A');
	});

	it('resolves a batch in order and de-duplicates lookups', () => {
		// Counting lookups matters: a forty-row table should not perform forty
		// independent resolutions against a table the caller already holds.
		let reads = 0;
		const counting = new Map<string, { name: string }>();
		for (const f of FACILITIES) counting.set(f.id, f);
		const originalGet = counting.get.bind(counting);
		counting.get = (key: string) => {
			reads += 1;
			return originalGet(key);
		};

		const labels = resolveMany(counting, ['f1', 'f1', 'f2', 'missing'], (f) => f.name);
		expect(labels).toEqual(['RS Pusat Kota', 'RS Pusat Kota', 'Puskesmas Timur', UNRESOLVED]);
		// Three distinct ids looked up once each; the repeated f1 is served from
		// the per-call cache, so four input ids cost three resolutions.
		expect(reads).toBe(3);
	});

	it('identifies the placeholder', () => {
		expect(isUnresolved(UNRESOLVED)).toBe(true);
		expect(isUnresolved('RS Pusat Kota')).toBe(false);
	});
});

/* ------------------------------------------------------------------ *
 * format
 * ------------------------------------------------------------------ */

const NOW = new Date('2026-09-26T12:00:00.000Z');

describe('format', () => {
	describe('relative time ordering', () => {
		/**
		 * The regression this section exists for: the unit must be chosen from
		 * the largest magnitude, and minutes must be tested before hours before
		 * days. A wrong order renders a two-day-old call as "48 jam lalu".
		 */
		it('uses minutes below an hour', () => {
			expect(formatRelativeTime('2026-09-26T11:45:00.000Z', NOW)).toBe('15 menit lalu');
			expect(formatRelativeTime('2026-09-26T11:59:00.000Z', NOW)).toBe('1 menit lalu');
		});

		it('uses hours below a day', () => {
			expect(formatRelativeTime('2026-09-26T10:00:00.000Z', NOW)).toBe('2 jam lalu');
			expect(formatRelativeTime('2026-09-26T11:00:00.000Z', NOW)).toBe('1 jam lalu');
		});

		it('uses days below a week', () => {
			expect(formatRelativeTime('2026-09-24T12:00:00.000Z', NOW)).toBe('2 hari lalu');
			expect(formatRelativeTime('2026-09-19T12:00:00.000Z', NOW)).toBe('7 hari lalu');
		});

		it('switches to an absolute date past a week rather than counting days', () => {
			// 47 days must not render as "47 hari lalu": that reads as current.
			const old = new Date(NOW);
			old.setDate(old.getDate() - 47);
			const formatted = formatRelativeTime(old, NOW);
			expect(formatted).not.toContain('hari lalu');
			expect(formatted).not.toContain('Baru saja');
		});

		it('treats a very recent timestamp as just now', () => {
			expect(formatRelativeTime('2026-09-26T11:59:50.000Z', NOW)).toBe('Baru saja');
		});

		it('does not render a future timestamp as a negative age', () => {
			// Clock skew must read as "just now", not "in -5 minutes".
			expect(formatRelativeTime('2026-09-26T12:05:00.000Z', NOW)).toBe('Baru saja');
		});

		it('renders an absent timestamp as the placeholder', () => {
			expect(formatRelativeTime(null, NOW)).toBe(EMPTY);
			expect(formatRelativeTime(undefined, NOW)).toBe(EMPTY);
			expect(formatRelativeTime('', NOW)).toBe(EMPTY);
			expect(formatRelativeTime('not-a-date', NOW)).toBe(EMPTY);
		});
	});

	describe('staleness', () => {
		it('flags a timestamp beyond the threshold as stale', () => {
			const old = new Date(NOW);
			old.setMinutes(old.getMinutes() - 10);
			expect(isStale(old, 5 * 60 * 1000, NOW)).toBe(true);
			expect(isStale(old, 30 * 60 * 1000, NOW)).toBe(false);
		});

		it('treats an absent timestamp as stale, never as fresh', () => {
			// Silently showing a blank timestamp as "current" is the failure this
			// guards against.
			expect(isStale(null, 60_000, NOW)).toBe(true);
		});
	});

	describe('absolute formatting', () => {
		it('formats dates and times in id-ID', () => {
			const value = '2026-09-26T07:05:00.000Z';
			// 07:05 UTC is 14.05 in WIB (UTC+7).
			expect(formatTime(value)).toMatch(/\d{2}[.:]\d{2}/);
			expect(formatDate(value)).toMatch(/2026/);
			expect(formatDateTime(value)).toMatch(/2026/);
		});

		it('groups numbers with Indonesian separators', () => {
			expect(formatNumber(1234567).replace(/\D/g, '')).toBe('1234567');
			expect(formatNumber(null)).toBe(EMPTY);
		});
	});

	describe('text helpers', () => {
		it('capitalises only the first letter', () => {
			expect(sentenceCase('menunggu')).toBe('Menunggu');
			expect(sentenceCase('  dalam pelayanan ')).toBe('Dalam pelayanan');
			expect(sentenceCase(null)).toBe(EMPTY);
		});

		it('truncates only when it actually cuts', () => {
			expect(truncate('abcdef', 10)).toBe('abcdef');
			expect(truncate('abcdef', 3)).toBe('ab…');
			expect(truncate('abcdef', 3)).toHaveLength(3);
		});
	});
});
