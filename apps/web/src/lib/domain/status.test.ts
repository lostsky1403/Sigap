import { describe, expect, it } from 'vitest';
import {
	APPOINTMENT_STATUSES,
	QUEUE_STATUSES,
	allowedAppointmentTransitions,
	allowedQueueTransitions,
	appointmentTone,
	canTransitionAppointment,
	canTransitionQueue,
	isActionableAppointmentStatus,
	isActionableQueueStatus,
	isTerminalAppointmentStatus,
	isTerminalQueueStatus,
	queueTone
} from './status';
import type { AppointmentStatus, QueueStatus } from '$lib/api/types/api';

/**
 * State machine contract (Phase 3B1 section 14).
 *
 * The assertions compare EXACT sets, not "contains". A subset check would pass
 * even if a transition were added by accident, and these tables exist precisely
 * because an illegal transition corrupts real data: closing a visit that was
 * never served, or resurrecting a cancelled ticket.
 */

describe('queue state machine', () => {
	it('allows exactly the documented transitions from waiting', () => {
		expect(allowedQueueTransitions('waiting')).toEqual(['called', 'cancelled']);
	});

	it('allows exactly the documented transitions from called', () => {
		expect(allowedQueueTransitions('called')).toEqual(['in_service', 'cancelled', 'skipped']);
	});

	it('allows exactly one transition from in_service', () => {
		expect(allowedQueueTransitions('in_service')).toEqual(['completed']);
	});

	it('treats completed, cancelled, and skipped as terminal', () => {
		expect(allowedQueueTransitions('completed')).toEqual([]);
		expect(allowedQueueTransitions('cancelled')).toEqual([]);
		expect(allowedQueueTransitions('skipped')).toEqual([]);
		expect(isTerminalQueueStatus('completed')).toBe(true);
		expect(isTerminalQueueStatus('cancelled')).toBe(true);
		expect(isTerminalQueueStatus('skipped')).toBe(true);
	});

	it('rejects a transition the server would reject', () => {
		// waiting -> in_service skips the call step entirely.
		expect(canTransitionQueue('waiting', 'in_service')).toBe(false);
		// A finished ticket cannot be reopened.
		expect(canTransitionQueue('completed', 'waiting')).toBe(false);
		expect(canTransitionQueue('cancelled', 'called')).toBe(false);
	});

	it('accepts every documented transition', () => {
		expect(canTransitionQueue('waiting', 'called')).toBe(true);
		expect(canTransitionQueue('waiting', 'cancelled')).toBe(true);
		expect(canTransitionQueue('called', 'in_service')).toBe(true);
		expect(canTransitionQueue('called', 'cancelled')).toBe(true);
		expect(canTransitionQueue('called', 'skipped')).toBe(true);
		expect(canTransitionQueue('in_service', 'completed')).toBe(true);
	});

	it('marks only non-terminal statuses actionable', () => {
		expect(isActionableQueueStatus('waiting')).toBe(true);
		expect(isActionableQueueStatus('called')).toBe(true);
		expect(isActionableQueueStatus('in_service')).toBe(true);
		expect(isActionableQueueStatus('completed')).toBe(false);
		expect(isActionableQueueStatus('cancelled')).toBe(false);
	});

	it('maps every status to a badge tone', () => {
		for (const status of QUEUE_STATUSES) {
			expect(['neutral', 'info', 'success', 'warning', 'danger']).toContain(
				queueTone(status as QueueStatus)
			);
		}
	});
});

describe('appointment state machine', () => {
	it('allows exactly the documented transitions from scheduled', () => {
		expect(allowedAppointmentTransitions('scheduled')).toEqual([
			'checked_in',
			'cancelled',
			'no_show'
		]);
	});

	it('allows exactly the documented transitions from checked_in', () => {
		expect(allowedAppointmentTransitions('checked_in')).toEqual([
			'queued',
			'cancelled',
			'no_show'
		]);
	});

	it('refuses checked_in -> completed', () => {
		// The transition that must not exist. Completing a visit without a
		// queue record means a patient left with no service evidence at all.
		expect(allowedAppointmentTransitions('checked_in')).not.toContain('completed');
		expect(canTransitionAppointment('checked_in', 'completed')).toBe(false);
	});

	it('allows exactly the documented transitions from queued', () => {
		expect(allowedAppointmentTransitions('queued')).toEqual([
			'completed',
			'cancelled',
			'no_show'
		]);
	});

	it('treats completed, cancelled, and no_show as terminal', () => {
		expect(allowedAppointmentTransitions('completed')).toEqual([]);
		expect(allowedAppointmentTransitions('cancelled')).toEqual([]);
		expect(allowedAppointmentTransitions('no_show')).toEqual([]);
		expect(isTerminalAppointmentStatus('no_show')).toBe(true);
	});

	it('rejects a transition the server would reject', () => {
		expect(canTransitionAppointment('scheduled', 'queued')).toBe(false);
		expect(canTransitionAppointment('scheduled', 'completed')).toBe(false);
		expect(canTransitionAppointment('completed', 'checked_in')).toBe(false);
	});

	it('accepts every documented transition', () => {
		expect(canTransitionAppointment('scheduled', 'checked_in')).toBe(true);
		expect(canTransitionAppointment('scheduled', 'cancelled')).toBe(true);
		expect(canTransitionAppointment('scheduled', 'no_show')).toBe(true);
		expect(canTransitionAppointment('checked_in', 'queued')).toBe(true);
		expect(canTransitionAppointment('queued', 'completed')).toBe(true);
	});

	it('maps every status to a badge tone', () => {
		for (const status of APPOINTMENT_STATUSES) {
			expect(['neutral', 'info', 'success', 'warning', 'danger']).toContain(
				appointmentTone(status as AppointmentStatus)
			);
		}
	});
});

describe('state machine completeness', () => {
	it('has an entry for every known status, with no extras', () => {
		expect([...QUEUE_STATUSES].sort()).toEqual(
			['waiting', 'called', 'in_service', 'completed', 'cancelled', 'skipped'].sort()
		);
		expect([...APPOINTMENT_STATUSES].sort()).toEqual(
			['scheduled', 'checked_in', 'queued', 'completed', 'cancelled', 'no_show'].sort()
		);
	});

	it('never lists a status as its own successor', () => {
		// A self-transition would let a UI re-save the same state and mask a
		// genuine backend rejection.
		for (const status of QUEUE_STATUSES) {
			expect(allowedQueueTransitions(status as QueueStatus)).not.toContain(status);
		}
		for (const status of APPOINTMENT_STATUSES) {
			expect(allowedAppointmentTransitions(status as AppointmentStatus)).not.toContain(status);
		}
	});
});
