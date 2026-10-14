/**
 * A polite live-region announcer.
 *
 * WHY A SEPARATE STORE FROM TOASTS. A toast is VISUAL: it appears, it goes
 * away, and a sighted operator reads it. An announcement is for a screen
 * reader, and it must survive the toast's lifetime — the two have different
 * consumers and different timing. Merging them would mean either the
 * announcement is cut short when the toast dismisses, or the toast's
 * visual presence is pinned by a screen-reader-only channel.
 *
 * WHY POLITE, NOT ASSERTIVE. An operator who has just clicked "Dipanggil"
 * already knows they clicked something; interrupting whatever a screen reader
 * is currently saying is hostile. `polite` queues the message and reads it at
 * the next natural pause, which is the difference between confirming an action
 * and startling someone.
 *
 * The message is a DOMAIN fact — "ticket QR-0001 moved to Dipanggil" — never a
 * capability claim. Nothing here knows or reports what the operator may do.
 */

/** How long a message stays in the live region before it is cleared. */
export const ANNOUNCE_CLEAR_MS = 6000;

type Subscriber = (message: string) => void;

let message = '';
let nextId = 1;
const subscribers = new Set<Subscriber>();
let clearTimer: ReturnType<typeof setTimeout> | undefined;

function emit(): void {
	for (const subscriber of subscribers) subscriber(message);
}

/**
 * Announces `text` politely, replacing anything currently queued.
 *
 * The id suffix is not decoration. Two identical announcements in quick
 * succession ("Antrean dipindahkan" twice) would be treated by a screen reader
 * as the SAME text and, on several of them, silently dropped — the operator
 * would hear one confirmation for two actions. The suffix makes each one
 * unique so both are read.
 *
 * An empty message is ignored rather than announced: clearing happens on a
 * timer, and announcing "" would queue a pause.
 */
export function announce(text: string): void {
	const trimmed = text.trim();
	if (!trimmed) return;

	message = `${trimmed}${nextId++}`;
	emit();

	if (clearTimer) clearTimeout(clearTimer);
	clearTimer = setTimeout(() => {
		clearTimer = undefined;
		message = '';
		emit();
	}, ANNOUNCE_CLEAR_MS);
}

/** Clears the region immediately. Used on navigation and by the test reset. */
export function clearAnnouncement(): void {
	if (clearTimer) {
		clearTimeout(clearTimer);
		clearTimer = undefined;
	}
	if (message === '') return;
	message = '';
	emit();
}

/** The current announcement, or '' when nothing is queued. */
export function getAnnouncement(): string {
	return message;
}

/** Subscribes, receiving the current message immediately. */
export function subscribeToAnnouncements(subscriber: Subscriber): () => void {
	subscribers.add(subscriber);
	subscriber(message);
	return () => {
		subscribers.delete(subscriber);
	};
}

/**
 * Wording for a successful status transition.
 *
 * Named after the queue and the appointment separately rather than taking a
 * generic subject, because an operator hearing "it moved" cannot tell which
 * thing moved. Both say what happened and nothing about permission.
 */
export function announceQueueTransition(
	formattedNumber: string,
	fromLabel: string,
	toLabel: string
): void {
	announce(`Antrean ${formattedNumber} dipindahkan dari ${fromLabel} ke ${toLabel}.`);
}

export function announceAppointmentTransition(
	patientName: string,
	fromLabel: string,
	toLabel: string
): void {
	announce(`Janji temu ${patientName} dipindahkan dari ${fromLabel} ke ${toLabel}.`);
}
