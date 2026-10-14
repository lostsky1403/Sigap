import type {
	AdminAppointment,
	AdminFacility,
	AdminQueueTicket,
	AdminSchedule
} from '$lib/api/types/api';

/**
 * Ringkasan's derived counts.
 *
 * EVERY figure on the overview is computed here, from the four already-loaded
 * scoped lists, and nowhere else. That constraint is the whole point of the
 * page. An operations dashboard is exactly where a number gets invented: a
 * percentage is easier than a ratio, a "trend" is easier than two points, and
 * both look authoritative next to real data.
 *
 * So this module returns counts and nothing else. There is no percentage, no
 * trend, no delta, no SLA, no occupancy rate, no chart series, and no
 * notification fallback. Each of those would require a denominator or a history
 * the scoped reads do not provide, and inventing one produces a number an
 * operator will act on.
 *
 * Every derived value also carries the SOURCE it came from, so the page can
 * label it. A count without provenance is a number the operator cannot check,
 * and an uncheckable number on a clinic dashboard is a liability.
 */

/** A count plus the list it was counted from. */
export interface DerivedMetric {
	/** The Indonesian label shown to the operator. */
	label: string;
	/** The number, derived only from the scoped list. */
	value: number;
	/** Which scoped read this came from, named for the UI. */
	source: string;
	/** Where the operator goes to act on it. */
	href?: string;
	/** A short qualifier, e.g. what the count includes. */
	detail?: string;
}

export interface RingkasanInput {
	queues: readonly AdminQueueTicket[];
	appointments: readonly AdminAppointment[];
	schedules: readonly AdminSchedule[];
	facilities: readonly AdminFacility[];
}

const ACTIVE_QUEUE_STATUSES = new Set(['waiting', 'called', 'in_service']);

export function activeQueueCount(queues: readonly AdminQueueTicket[]): number {
	// "Active" is waiting + called + in_service: everything not yet closed. A
	// completed or cancelled ticket is history, and counting it would make a
	// quiet afternoon look like a backlog.
	return queues.filter((ticket) => ACTIVE_QUEUE_STATUSES.has(ticket.status)).length;
}

export function scheduledAppointmentCount(appointments: readonly AdminAppointment[]): number {
	return appointments.filter((appointment) => appointment.status === 'scheduled').length;
}

export function activeScheduleCount(schedules: readonly AdminSchedule[]): number {
	return schedules.filter((schedule) => schedule.is_active).length;
}

export function activeFacilityCount(facilities: readonly AdminFacility[]): number {
	return facilities.filter((facility) => facility.is_active).length;
}

/**
 * The headline strip. Three figures, each traceable to one scoped list.
 *
 * Notification counts are deliberately ABSENT. The outbox summary is scoped to
 * the operator's own facilities but counts delivery attempts, not clinic
 * activity, so placing it beside "active queue" would imply a relationship the
 * two numbers do not have. An operator would reasonably read them as comparable.
 */
export function headlineMetrics(input: RingkasanInput): DerivedMetric[] {
	return [
		{
			label: 'Antrean aktif',
			value: activeQueueCount(input.queues),
			source: 'Daftar antrean',
			detail: 'Menunggu, dipanggil, dan sedang dilayani',
			href: '/admin/queues'
		},
		{
			label: 'Janji temu terjadwal',
			value: scheduledAppointmentCount(input.appointments),
			source: 'Daftar janji temu',
			detail: 'Status terjadwal',
			href: '/admin/appointments'
		},
		{
			label: 'Fasilitas aktif',
			value: activeFacilityCount(input.facilities),
			source: 'Daftar fasilitas',
			detail: 'Fasilitas yang masih menerima pasien',
			href: '/admin/facilities'
		}
	];
}

/**
 * The "needs attention" rows.
 *
 * Only counts, and only ones the scoped lists can honestly produce. A clinic
 * average wait, a no-show rate, or a bed occupancy percentage would each need a
 * denominator or a history that these four lists do not carry, so they are not
 * here. The absence is deliberate and the page says so.
 */
export function attentionItems(input: RingkasanInput): DerivedMetric[] {
	const items: DerivedMetric[] = [];

	const waiting = input.queues.filter((ticket) => ticket.status === 'waiting').length;
	if (waiting > 0) {
		items.push({
			label: 'Antrean menunggu belum dipanggil',
			value: waiting,
			source: 'Daftar antrean',
			href: '/admin/queues'
		});
	}

	// Notification failures are deliberately NOT listed here. The outbox summary
	// is a delivery-attempt count scoped to the operator's facilities, not a
	// clinic-activity figure, and putting "notifications failed" beside "queue
	// waiting" would imply the two are comparable. Ringkasan does not load the
	// outbox, so any figure here would be invented — and a hardcoded zero would
	// be the most dangerous kind of invented, because it asserts that nothing has
	// ever failed. An operator who trusts that zero stops checking the outbox.
	const noShow = input.appointments.filter(
		(appointment) => appointment.status === 'no_show'
	).length;
	if (noShow > 0) {
		items.push({
			label: 'Janji temu tidak datang',
			value: noShow,
			source: 'Daftar janji temu',
			href: '/admin/appointments'
		});
	}

	return items;
}

/**
 * The next active schedule, in the scope.
 *
 * "Next" is decided by date and then start time. A schedule whose date cannot be
 * parsed is skipped rather than sorted to the top: an unparseable date means the
 * record is malformed, and putting it first would present the least trustworthy
 * row as the most urgent.
 */
export function nextSchedule(
	schedules: readonly AdminSchedule[]
): AdminSchedule | null {
	const today = new Date();
	const todayKey = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(
		today.getDate()
	).padStart(2, '0')}`;

	const upcoming = schedules
		.filter((schedule) => schedule.is_active)
		.filter((schedule) => /^\d{4}-\d{2}-\d{2}$/.test(schedule.schedule_date))
		.filter((schedule) => schedule.schedule_date >= todayKey)
		.sort((a, b) =>
			a.schedule_date === b.schedule_date
				? a.start_time.localeCompare(b.start_time)
				: a.schedule_date.localeCompare(b.schedule_date)
		);

	return upcoming[0] ?? null;
}
