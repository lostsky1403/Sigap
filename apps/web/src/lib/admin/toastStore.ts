/**
 * A tiny toast queue for admin mutation feedback.
 *
 * WHY A STORE. Phase 3B4 left `Toast.svelte` built, tested, and unused: nothing
 * in the app had a reason to raise one. Mutations are that reason, and a
 * mutation can be raised from a page, a table row, or an editor dialog — three
 * different components that must not each own a `<Toast>` and a timer. A store
 * is the smallest thing that lets all three say "that worked" without knowing
 * about each other.
 *
 * WHY IT IS NOT AN AUTHORIZATION MECHANISM. A toast says what happened. It
 * never says what the actor may do, and nothing here reads a permission, a
 * role, or a scope. The mutation endpoint already decided; this only narrates
 * the decision it returned.
 *
 * TIMING. `SUCCESS_DURATION_MS` is 4s. Long enough to read a sentence, short
 * enough that a sequence of mutations does not build a wall of toasts. Errors
 * are deliberately NOT queued here — they are rendered inline next to the
 * control that failed, where the operator can act on them, and a toast that
 * disappears takes the explanation with it.
 */

/** How long a success toast stays up. */
export const SUCCESS_DURATION_MS = 4000;

/** How long a warning toast stays up. */
export const WARNING_DURATION_MS = 6000;

export type ToastVariant = 'success' | 'warning' | 'info';

export interface AdminToast {
	/** Monotonic id, so two identical messages in a row are distinct entries. */
	id: number;
	message: string;
	title: string;
	variant: ToastVariant;
	duration: number;
}

type Subscriber = (toasts: readonly AdminToast[]) => void;

let toasts: AdminToast[] = [];
let nextId = 1;
const subscribers = new Set<Subscriber>();

function emit(): void {
	// A frozen snapshot, so a consumer holding a stale reference does not see
	// the array change underneath it. This is the same immutability discipline
	// the session store uses, and for the same reason: two panels can watch the
	// queue without coordinating.
	const snapshot: readonly AdminToast[] = Object.freeze([...toasts]);
	for (const subscriber of subscribers) subscriber(snapshot);
}

/**
 * Raises a toast and returns its id.
 *
 * `duration <= 0` keeps it until dismissed, which is how a caller pins a
 * message that must not vanish before it is read.
 */
export function pushToast(
	message: string,
	{
		title = '',
		variant = 'success',
		duration = SUCCESS_DURATION_MS
	}: { title?: string; variant?: ToastVariant; duration?: number } = {}
): number {
	const id = nextId++;
	toasts = [...toasts, { id, message, title, variant, duration }];
	emit();
	return id;
}

/** Dismisses one toast by id. Unknown ids are ignored, not an error. */
export function dismissToast(id: number): void {
	const next = toasts.filter((toast) => toast.id !== id);
	if (next.length === toasts.length) return;
	toasts = next;
	emit();
}

/** Dismisses every toast. Used on navigation and by the test reset. */
export function clearToasts(): void {
	if (toasts.length === 0) return;
	toasts = [];
	emit();
}

/** The current queue. Read-only by type; use the functions above to change it. */
export function getToasts(): readonly AdminToast[] {
	return toasts;
}

/**
 * Subscribes to the queue, receiving the current state immediately.
 *
 * The catch-up call is deliberate: a component that mounts while a toast is
 * already showing must render it, or a mutation that fired before the panel
 * mounted would leave the operator with no confirmation at all.
 */
export function subscribeToToasts(subscriber: Subscriber): () => void {
	subscribers.add(subscriber);
	subscriber(Object.freeze([...toasts]));
	return () => {
		subscribers.delete(subscriber);
	};
}

/**
 * Builds the success message for a status transition.
 *
 * Kept beside the transition tables so the wording cannot describe a change the
 * table does not permit. "Antrean dipindahkan ke Selesai" is a claim about what
 * happened, made only after the server agreed.
 */
export function transitionToastMessage(
	subject: string,
	fromLabel: string,
	toLabel: string
): string {
	return `${subject} dipindahkan dari ${fromLabel} ke ${toLabel}.`;
}
