import type { AppointmentStatus, QueueStatus } from '$lib/api/types/api';

/**
 * Status state machines.
 *
 * These mirror `validQueueTransitions` and `validAppointmentTransitions` in
 * apps/api/internal/handler/admin.go exactly. The server enforces the truth; this
 * copy exists so the UI can hide an action the server would reject, not to
 * decide it. Offering a button that always answers 409 is worse than not
 * offering it, so the sets below must stay identical to the Go maps.
 *
 * Note what is absent: there is no `checked_in -> completed`. A citizen who has
 * checked in must pass through `queued` before completing, because completing
 * is what actually closes a visit. A transition table that allowed the shortcut
 * would let a visit be closed with no service record at all.
 */

const QUEUE_TRANSITIONS: Record<QueueStatus, readonly QueueStatus[]> = {
	waiting: ['called', 'cancelled'],
	called: ['in_service', 'cancelled', 'skipped'],
	in_service: ['completed'],
	// Terminal. These appear as keys so the lookup never returns undefined, but
	// the empty array is the real statement: nothing follows them.
	completed: [],
	cancelled: [],
	skipped: []
};

const APPOINTMENT_TRANSITIONS: Record<AppointmentStatus, readonly AppointmentStatus[]> = {
	scheduled: ['checked_in', 'cancelled', 'no_show'],
	checked_in: ['queued', 'cancelled', 'no_show'],
	queued: ['completed', 'cancelled', 'no_show'],
	completed: [],
	cancelled: [],
	no_show: []
};

/** Statuses a queue ticket can be in. */
export const QUEUE_STATUSES = Object.keys(QUEUE_TRANSITIONS) as QueueStatus[];

/** Statuses an appointment can be in. */
export const APPOINTMENT_STATUSES = Object.keys(
	APPOINTMENT_TRANSITIONS
) as AppointmentStatus[];

/** Statuses with no onward transition. */
export const TERMINAL_QUEUE_STATUSES: readonly QueueStatus[] = ['completed', 'cancelled', 'skipped'];
export const TERMINAL_APPOINTMENT_STATUSES: readonly AppointmentStatus[] = [
	'completed',
	'cancelled',
	'no_show'
];

/** The exact set of statuses reachable from a queue status. Empty if unknown. */
export function allowedQueueTransitions(status: QueueStatus): readonly QueueStatus[] {
	return QUEUE_TRANSITIONS[status] ?? [];
}

/** The exact set of statuses reachable from an appointment status. */
export function allowedAppointmentTransitions(
	status: AppointmentStatus
): readonly AppointmentStatus[] {
	return APPOINTMENT_TRANSITIONS[status] ?? [];
}

/** True only when the server would accept this queue transition. */
export function canTransitionQueue(from: QueueStatus, to: QueueStatus): boolean {
	return allowedQueueTransitions(from).includes(to);
}

/** True only when the server would accept this appointment transition. */
export function canTransitionAppointment(
	from: AppointmentStatus,
	to: AppointmentStatus
): boolean {
	return allowedAppointmentTransitions(from).includes(to);
}

export function isTerminalQueueStatus(status: QueueStatus): boolean {
	return TERMINAL_QUEUE_STATUSES.includes(status);
}

export function isTerminalAppointmentStatus(status: AppointmentStatus): boolean {
	return TERMINAL_APPOINTMENT_STATUSES.includes(status);
}

/**
 * The read-only statuses an admin sees for a queue ticket.
 *
 * A ticket that is in service can still be cancelled or skipped from the
 * `called` state, so this is about whether further action is possible at all,
 * not about which single action is expected next.
 */
export function isActionableQueueStatus(status: QueueStatus): boolean {
	return !isTerminalQueueStatus(status);
}

export function isActionableAppointmentStatus(status: AppointmentStatus): boolean {
	return !isTerminalAppointmentStatus(status);
}

/**
 * Tone for a StatusBadge, derived from the status rather than passed in.
 *
 * Keeping the mapping here means a status cannot render green in one table and
 * amber in another, which is the kind of drift that makes a colour-coded board
 * unreadable.
 */
export function queueTone(status: QueueStatus): 'neutral' | 'info' | 'success' | 'warning' | 'danger' {
	switch (status) {
		case 'waiting':
			return 'neutral';
		case 'called':
			return 'info';
		case 'in_service':
			return 'warning';
		case 'completed':
			return 'success';
		case 'cancelled':
		case 'skipped':
			return 'danger';
	}
}

export function appointmentTone(
	status: AppointmentStatus
): 'neutral' | 'info' | 'success' | 'warning' | 'danger' {
	switch (status) {
		case 'scheduled':
			return 'neutral';
		case 'checked_in':
			return 'info';
		case 'queued':
			return 'warning';
		case 'completed':
			return 'success';
		case 'cancelled':
		case 'no_show':
			return 'danger';
	}
}

/** Indonesian label for a queue status, as shown to staff and citizens. */
export const QUEUE_STATUS_LABEL: Record<QueueStatus, string> = {
	waiting: 'Menunggu',
	called: 'Dipanggil',
	in_service: 'Dalam Pelayanan',
	completed: 'Selesai',
	cancelled: 'Dibatalkan',
	skipped: 'Dilewati'
};

export const APPOINTMENT_STATUS_LABEL: Record<AppointmentStatus, string> = {
	scheduled: 'Terjadwal',
	checked_in: 'Sudah Check-in',
	queued: 'Dalam Antrean',
	completed: 'Selesai',
	cancelled: 'Dibatalkan',
	no_show: 'Tidak Datang'
};
