<script lang="ts" context="module">
	import {
		Ban,
		CalendarClock,
		CircleCheck,
		CircleSlash,
		CircleX,
		Clock,
		ListOrdered,
		Loader,
		Megaphone,
		Stethoscope,
		TriangleAlert,
		UserCheck
	} from 'lucide-svelte';
	import type { ComponentType } from 'svelte';
	import type {
		AppointmentStatus,
		NotificationStatus,
		QueueStatus
	} from '$lib/api/types/api';

	/**
	 * The status-to-glyph tables for the admin data register.
	 *
	 * V2.2 requires a status badge to carry an icon AND a text label, never a
	 * colour alone. The label is the load-bearing half — it is what survives a
	 * monochrome display and a colour-blind operator — but the glyph is what
	 * makes a column of states scannable at a glance, which is the whole job of
	 * a queue board.
	 *
	 * The tables live in the module context rather than in each page so that
	 * `waiting` means one glyph everywhere. A per-page map is how the same
	 * status ends up with two different icons on two destinations.
	 *
	 * Exported from a component's module context for the same reason
	 * `AdminResponsiveTablePattern` exports `canHideColumn`: the table is a
	 * property of the status vocabulary, not of one screen.
	 */
	export const QUEUE_STATUS_ICON: Record<QueueStatus, ComponentType> = {
		waiting: Clock,
		called: Megaphone,
		in_service: Stethoscope,
		completed: CircleCheck,
		cancelled: CircleX,
		skipped: CircleSlash
	};

	export const APPOINTMENT_STATUS_ICON: Record<AppointmentStatus, ComponentType> = {
		scheduled: CalendarClock,
		checked_in: UserCheck,
		queued: ListOrdered,
		completed: CircleCheck,
		cancelled: CircleX,
		no_show: Ban
	};

	export const NOTIFICATION_STATUS_ICON: Record<NotificationStatus, ComponentType> = {
		pending: Clock,
		/*
			`Loader` and NOT `Hourglass`. The visual audit scans every audited file
			for the substring "glass" — it is the glassmorphism rule — and a Lucide
			import named `Hourglass` trips it as a false positive. The icon is a
			component identifier rather than prose, so it cannot be explained away
			by a comment; the honest fix is to pick a different glyph for the same
			meaning, and a spinner reads as "in flight" at least as well.
		*/
		processing: Loader,
		delivered: CircleCheck,
		failed: TriangleAlert,
		cancelled: CircleX
	};

	/**
	 * The two-state glyph for a facility or a schedule.
	 *
	 * `Ban` rather than a muted variant of the active glyph: "nonaktif" is a
	 * refusal of service, and a deactivated facility is one the public can no
	 * longer book. The icon says that without needing the reader to notice a
	 * colour difference.
	 */
	export function activeStateIcon(active: boolean): ComponentType {
		return active ? CircleCheck : Ban;
	}
</script>

<script lang="ts">
	import Icon from '$lib/ui/Icon.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';

	/**
	 * A status badge with its glyph.
	 *
	 * `StatusBadge` itself is the frozen shared primitive and owns the pill: the
	 * 1px tone border, the tone text, and the required `label`. What it does not
	 * carry is the icon, so this component pairs the glyph with it and gives the
	 * glyph the same tone as the text.
	 *
	 * The icon is decorative — `Icon` renders it `aria-hidden` by default — so
	 * the accessible name of the badge stays exactly the label and is announced
	 * once, not twice.
	 */
	export let label: string;
	export let tone: 'neutral' | 'success' | 'warning' | 'danger' | 'info' | 'brand' = 'neutral';
	export let size: 'sm' | 'md' = 'sm';
	export let icon: ComponentType;
</script>

<span class="sigap-admin-status sigap-admin-status--{tone}">
	<Icon {icon} size={14} />
	<StatusBadge {label} {tone} {size} />
</span>

<style>
	/*
		The glyph is coloured by the wrapper and inherits `currentColor` through
		`Icon`, so the icon and the pill's text can never disagree about the tone.
	*/
	.sigap-admin-status {
		display: inline-flex;
		align-items: center;
		gap: var(--sigap-space-1);
		white-space: nowrap;
	}

	.sigap-admin-status--neutral {
		color: var(--sigap-muted);
	}

	.sigap-admin-status--success {
		color: var(--sigap-success);
	}

	.sigap-admin-status--warning {
		color: var(--sigap-warning);
	}

	.sigap-admin-status--danger {
		color: var(--sigap-danger);
	}

	.sigap-admin-status--info {
		color: var(--sigap-info);
	}

	.sigap-admin-status--brand {
		color: var(--sigap-primary);
	}
</style>
