import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { RefreshCw, X } from 'lucide-svelte';
import { CONTROL_HEIGHT } from '$lib/design/tokens';
import Button from './Button.svelte';
import IconButton from './IconButton.svelte';
import { DENSITY, controlHeight, toDensity } from './density';

describe('Button', () => {
	it('renders a real button element', () => {
		render(Button, { props: { label: 'Simpan' } });
		const button = screen.getByRole('button');
		expect(button.tagName).toBe('BUTTON');
		// A div with a click handler would have no implicit role at all.
		expect(button).toHaveAttribute('type', 'button');
	});

	it('is focusable and activates on Enter and Space', async () => {
		const user = userEvent.setup();
		const onClick = vi.fn();
		render(Button, { props: { label: 'Simpan', onClick } });

		await user.tab();
		expect(screen.getByRole('button')).toHaveFocus();

		await user.keyboard('{Enter}');
		await user.keyboard(' ');
		expect(onClick).toHaveBeenCalledTimes(2);
	});

	it('exposes disabled semantics and does not fire', async () => {
		const user = userEvent.setup();
		const onClick = vi.fn();
		render(Button, { props: { label: 'Simpan', disabled: true, onClick } });

		const button = screen.getByRole('button');
		expect(button).toBeDisabled();
		// aria-disabled mirrors the native attribute for assistive tech that
		// reads the state rather than the behaviour.
		expect(button).toHaveAttribute('aria-disabled', 'true');

		await user.click(button);
		expect(onClick).not.toHaveBeenCalled();
	});

	it('associates a disabled reason with the control', () => {
		render(Button, {
			props: { label: 'Simpan', disabled: true, disabledReason: 'Isi nama dahulu' }
		});
		const button = screen.getByRole('button');
		const describedBy = button.getAttribute('aria-describedby');
		expect(describedBy).toBeTruthy();
		expect(document.getElementById(describedBy as string)).toHaveTextContent('Isi nama dahulu');
	});

	it('gives an icon-only button an accessible name', () => {
		render(Button, { props: { label: 'Muat ulang', icon: RefreshCw } });
		expect(screen.getByRole('button', { name: 'Muat ulang' })).toBeInTheDocument();
	});

	it('honors the frozen density heights', () => {
		for (const density of [DENSITY.citizen, DENSITY.adminCompact, DENSITY.adminComfortable]) {
			const { container, unmount } = render(Button, { props: { label: 'X', size: density } });
			const button = container.querySelector('button') as HTMLElement;
			expect(button.style.height).toBe(`${controlHeight(density)}px`);
			unmount();
		}
	});

	it('never renders a citizen control below 44px', () => {
		const { container } = render(Button, { props: { label: 'X', size: DENSITY.citizen } });
		const height = Number.parseInt(
			(container.querySelector('button') as HTMLElement).style.height,
			10
		);
		expect(height).toBeGreaterThanOrEqual(CONTROL_HEIGHT.citizen);
	});
});

describe('IconButton', () => {
	it('requires and applies an accessible name', () => {
		render(IconButton, { props: { icon: X, label: 'Tutup' } });
		const button = screen.getByRole('button', { name: 'Tutup' });
		expect(button).toBeInTheDocument();
		expect(button.tagName).toBe('BUTTON');
	});

	it('matches the density target size', () => {
		const { container } = render(IconButton, {
			props: { icon: X, label: 'Tutup', size: DENSITY.citizen }
		});
		const button = container.querySelector('button') as HTMLElement;
		expect(button.style.width).toBe('44px');
		expect(button.style.height).toBe('44px');
	});
});

describe('density', () => {
	it('maps each density to its frozen control height', () => {
		expect(controlHeight(DENSITY.citizen)).toBe(CONTROL_HEIGHT.citizen);
		expect(controlHeight(DENSITY.adminCompact)).toBe(CONTROL_HEIGHT.adminCompact);
		expect(controlHeight(DENSITY.adminComfortable)).toBe(CONTROL_HEIGHT.adminComfortable);
	});

	it('supports both admin densities', () => {
		expect(controlHeight(DENSITY.adminCompact)).toBeGreaterThanOrEqual(36);
		expect(controlHeight(DENSITY.adminComfortable)).toBeGreaterThanOrEqual(40);
	});

	it('falls back to citizen when the density is unknown', () => {
		// Fail toward the larger target: a citizen form silently rendering at
		// 36px is an accessibility bug, an admin control at 44px is just roomy.
		expect(toDensity('nonsense')).toBe(DENSITY.citizen);
		expect(toDensity(undefined)).toBe(DENSITY.citizen);
		expect(toDensity(DENSITY.adminCompact)).toBe(DENSITY.adminCompact);
	});
});
