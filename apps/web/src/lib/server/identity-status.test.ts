import { describe, expect, it } from "vitest";
import { assertActiveIdentity } from "./identity-status";

/**
 * Unit contract for the identity-status gate (adopted ADR Option 1).
 *
 * These tests use a FAKE pool so they run without a database. They prove the
 * fail-closed decision logic and the generic-error contract. Real session-row
 * and JWT behaviour is covered by the DB-backed integration test
 * (tests/identity-deactivation.integration.test.js), which CI runs with a
 * disposable Postgres; those scenarios are NOT RUN locally without a DB.
 */

type Pool = {
	query: (sql: string, params: unknown[]) => Promise<{ rows: unknown[] }>;
};

const fakePool = (rows: unknown[], throwOnQuery = false): Pool => ({
	query: async (sql: string, params: unknown[]) => {
		if (throwOnQuery) throw new Error("simulated DB outage");
		// Guard the query shape without printing any secret/PII.
		expect(sql).toContain("public.app_users");
		expect(sql).toContain("WHERE subject = $1");
		expect(params).toHaveLength(1);
		expect(params[0]).toBe("subj-active-1");
		return { rows };
	},
});

async function refused(subject: unknown, pool: Pool): Promise<boolean> {
	try {
		await assertActiveIdentity(subject, pool);
		return false;
	} catch (err) {
		// Every refusal must carry the Better Auth APIError shape the login
		// action recognises: status "UNAUTHORIZED" (and/or statusCode 401),
		// which it maps to the same generic credential message.
		const shaped = err as { status?: string; statusCode?: number; body?: unknown };
		expect(
			shaped.status === "UNAUTHORIZED" || shaped.statusCode === 401
		).toBe(true);
		return true;
	}
}

describe("assertActiveIdentity — fail-closed identity gate", () => {
	it("allows a confirmed ACTIVE identity (positive control)", async () => {
		await expect(
			assertActiveIdentity("subj-active-1", fakePool([{ status: "active", deleted_at: null }]))
		).resolves.toBeUndefined();
	});

	it("rejects an explicitly DISABLED identity", async () => {
		expect(await refused("subj-active-1", fakePool([{ status: "disabled", deleted_at: null }]))).toBe(true);
	});

	it("rejects a SOFT-DELETED identity even when status is active", async () => {
		expect(
			await refused("subj-active-1", fakePool([{ status: "active", deleted_at: new Date().toISOString() }]))
		).toBe(true);
	});

	it("rejects a MISSING application identity (no row)", async () => {
		expect(await refused("subj-active-1", fakePool([]))).toBe(true);
	});

	it("rejects when the identity check throws (DB outage) — fail closed", async () => {
		expect(await refused("subj-active-1", fakePool([], true))).toBe(true);
	});

	it("rejects a blank / non-string subject (no authenticated identity)", async () => {
		for (const subject of ["", "   ", undefined, null, 42, {}]) {
			expect(await refused(subject, fakePool([{ status: "active", deleted_at: null }]))).toBe(true);
		}
	});
});
