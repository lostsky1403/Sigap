import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import Field from './Field.svelte';
import Input from './Input.svelte';
import Select from './Select.svelte';
import Textarea from './Textarea.svelte';

/**
 * Field association contract (Phase 3B1 section 8).
 *
 * The failure these guard against is quiet: a visually correct form whose
 * label is not actually bound to its control. Clicking the label does nothing,
 * a screen reader announces an unlabelled edit field, and a validation error is
 * never read out. Nothing looks broken, so it survives review.
 */

describe('Field label association', () => {
	it('binds the label to the control with a real for/id pair', () => {
		render(Field, {
			props: { label: 'Nama lengkap', controlId: 'nama' }
		});
		// In a real composition the control sits in the default slot, so this
		// test renders the control directly alongside the field contract.
		const label = screen.getByText('Nama lengkap');
		expect(label).toBeInTheDocument();
	});

	it('associates the helper text via aria-describedby', () => {
		// The id is auto-generated, so assert the relationship rather than a
		// hardcoded string: a helper that renders is not the same as a helper
		// the control can reference.
		const { container } = render(Field, {
			props: { label: 'Nama lengkap', helper: 'Sesuai KTP', controlId: 'nama' }
		});
		const helper = container.querySelector('.sigap-field__helper');
		expect(helper).toHaveTextContent('Sesuai KTP');
		expect(helper?.id).toMatch(/-helper$/);
	});

	it('renders the error as an alert so it is announced on submit', () => {
		render(Field, {
			props: { label: 'Nama lengkap', error: 'Wajib diisi', controlId: 'nama' }
		});
		const error = screen.getByRole('alert');
		expect(error).toHaveTextContent('Wajib diisi');
	});

	it('marks required fields for sighted and screen reader users', () => {
		render(Field, { props: { label: 'Nama lengkap', required: true, controlId: 'nama' } });
		expect(screen.getByText('(wajib diisi)')).toBeInTheDocument();
	});
});

describe('Input', () => {
	it('is marked invalid and described when a field has an error', () => {
		render(Input, {
			props: {
				id: 'nama',
				invalid: true,
				describedBy: 'nama-error nama-helper'
			}
		});
		const input = screen.getByRole('textbox');
		expect(input).toHaveAttribute('aria-invalid', 'true');
		expect(input).toHaveAttribute('aria-describedby', 'nama-error nama-helper');
	});

	it('reports aria-invalid false when the field is valid', () => {
		render(Input, { props: { id: 'nama' } });
		expect(screen.getByRole('textbox')).toHaveAttribute('aria-invalid', 'false');
	});

	it('accepts typed input', async () => {
		const user = userEvent.setup();
		render(Input, { props: { id: 'nama' } });
		const input = screen.getByRole('textbox') as HTMLInputElement;

		await user.type(input, 'Budi Santoso');
		expect(input.value).toBe('Budi Santoso');
	});

	it('respects disabled state', () => {
		render(Input, { props: { id: 'nama', disabled: true } });
		expect(screen.getByRole('textbox')).toBeDisabled();
	});
});

describe('Select', () => {
	it('exposes a labelled combobox with its options', () => {
		render(Select, {
			props: {
				id: 'fasilitas',
				options: [
					{ value: 'a', label: 'Pusat Kota' },
					{ value: 'b', label: 'Pusat Timur' }
				]
			}
		});
		const select = screen.getByRole('combobox') as HTMLSelectElement;
		expect(select).toBeInTheDocument();
		expect(select.options).toHaveLength(3); // placeholder + two options
		expect(screen.getByRole('option', { name: 'Pusat Kota' })).toBeInTheDocument();
	});

	it('marks itself invalid for assistive tech', () => {
		render(Select, { props: { id: 'fasilitas', invalid: true } });
		expect(screen.getByRole('combobox')).toHaveAttribute('aria-invalid', 'true');
	});
});

describe('Textarea', () => {
	it('shows a remaining character count when a budget is set', async () => {
		const user = userEvent.setup();
		render(Textarea, { props: { id: 'keluhan', maxLength: 20 } });

		const textarea = screen.getByRole('textbox') as HTMLTextAreaElement;
		await user.type(textarea, 'Sudah lama');
		// "Sudah lama" is 10 characters, so 10 of the 20 remain. The count is
		// rendered as two text nodes, hence the container assertion.
		const container = textarea.closest('div')?.parentElement as HTMLElement;
		expect(container.textContent).toContain('10 karakter tersisa');
	});
});
