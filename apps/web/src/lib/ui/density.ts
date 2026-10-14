/**
 * Control density — the one place the two SIGAP densities are decided.
 *
 * SIGAP has one brand and two densities. Citizen surfaces are touch-first and
 * must clear 44px. Admin surfaces are denser at 36px compact and 40px
 * comfortable, because an admin working a queue all day benefits from more rows
 * per screen.
 *
 * Primitives accept `density` rather than a raw pixel height. That is
 * intentional: it makes "make this 38px" impossible to express, so the citizen
 * floor cannot be quietly broken by an admin-oriented change.
 */
export const DENSITY = {
	/** Touch-first. 44px minimum, per accessibility guidance for citizen forms. */
	citizen: 'citizen',
	/** Dense admin tables and toolbars. */
	adminCompact: 'admin-compact',
	/** Slightly roomier admin controls for primary actions. */
	adminComfortable: 'admin-comfortable'
} as const;

export type Density = (typeof DENSITY)[keyof typeof DENSITY];

export const DENSITIES: readonly Density[] = [
	DENSITY.citizen,
	DENSITY.adminCompact,
	DENSITY.adminComfortable
];

/** The frozen control height in pixels for a density. */
export function controlHeight(density: Density): number {
	switch (density) {
		case DENSITY.citizen:
			return 44;
		case DENSITY.adminCompact:
			return 36;
		case DENSITY.adminComfortable:
			return 40;
	}
}

/** Text size that stays legible at the given density. */
export function controlFontSize(density: Density): string {
	return density === DENSITY.citizen ? '16px' : '14px';
}

/** Horizontal padding, kept on the 8px spacing rhythm. */
export function controlPaddingX(density: Density): string {
	return density === DENSITY.citizen ? '16px' : '12px';
}

/**
 * Narrows an arbitrary string to a Density, defaulting to citizen.
 *
 * The default is the safe direction: when density cannot be determined, the
 * larger touch target wins. A citizen form silently rendering at 36px is an
 * accessibility bug, whereas an admin control rendering at 44px is merely
 * roomy.
 */
export function toDensity(value: string | null | undefined): Density {
	return DENSITIES.includes(value as Density) ? (value as Density) : DENSITY.citizen;
}
