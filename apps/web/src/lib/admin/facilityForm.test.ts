import { describe, expect, it } from 'vitest';
import {
	blankFacility,
	FORBIDDEN_PHONE_CHARACTERS,
	hasForbiddenPhoneCharacter,
	isFacilityFormValid,
	validateFacilityForm,
	type FacilityFormValues
} from './facilityForm';

/**
 * T-3B5 §12: the facility form's own validation.
 *
 * Two rules carry the weight here, and they fail in opposite directions.
 *
 * The REQUIRED-FIELD rule exists so the operator is told which field is missing
 * before a round trip that would discard everything they typed. Its failure
 * mode is over-strictness: demanding the full create set on an UPDATE would
 * block a legitimate one-field rename, which is exactly what the update path
 * exists to allow. So the two modes are asserted separately — a rule that
 * applies to both is a bug waiting to happen.
 *
 * The PHONE-CHARACTER rule exists because Go rejects a phone containing any of
 * `< > " ' &`. That is a real server rule with a real reason (those characters
 * appear in injection payloads), so the client has to know it too. Asserting
 * only "some bad input is rejected" would pass against a rule with the wrong
 * character set, which would then reject a perfectly good number and accept a
 * genuinely dangerous one.
 */

/** A create form that is valid in every respect, so each case breaks one rule. */
function validForm(overrides: Partial<FacilityFormValues> = {}): FacilityFormValues {
	return {
		...blankFacility(),
		name: 'Klinik Uji',
		type: 'puskesmas',
		address: 'Jl. Uji No. 1',
		kecamatan: 'Cibinong',
		kabupaten_kota: 'Bogor',
		provinsi: 'Jawa Barat',
		phone: '08123456789',
		total_beds: '12',
		available_beds: '7',
		short_code: 'KUJ',
		...overrides
	};
}

describe('the blank form is not valid, and names what is missing', () => {
	it('refuses an entirely empty create form', () => {
		const errors = validateFacilityForm(blankFacility(), 'create');
		// Every required field is reported at once. One-at-a-time reporting would
		// make the operator fix a seven-field form in seven round trips.
		expect(Object.keys(errors).sort()).toEqual(
			['address', 'name', 'phone', 'short_code', 'type'].sort()
		);
		expect(isFacilityFormValid(blankFacility(), 'create')).toBe(false);
	});

	it('treats whitespace as missing rather than as a value', () => {
		// A field of spaces satisfies a `trim() === ''` check and defeats a naive
		// `=== ''` one, which would let the server reject an empty name instead.
		expect(validateFacilityForm(validForm({ name: '   ' }), 'create').name).toBe(
			'Nama fasilitas wajib diisi.'
		);
		expect(validateFacilityForm(validForm({ phone: '  ' }), 'create').phone).toBe(
			'Nomor telepon wajib diisi.'
		);
	});

	it('accepts a fully populated create form', () => {
		expect(validateFacilityForm(validForm(), 'create')).toEqual({});
		expect(isFacilityFormValid(validForm(), 'create')).toBe(true);
	});
});

describe('create demands the full required set', () => {
	it('names the address group as one field, because the operator can see all four', () => {
		// Naming which of the four was missed would be a guess.
		const errors = validateFacilityForm(
			validForm({ kecamatan: '', provinsi: '' }),
			'create'
		);
		expect(errors.address).toBe(
			'Alamat lengkap (jalan, kecamatan, kabupaten/kota, provinsi) wajib diisi.'
		);
	});

	it('requires the type, and rejects one outside the enum', () => {
		expect(validateFacilityForm(validForm({ type: '' }), 'create').type).toBeDefined();
		expect(
			validateFacilityForm(validForm({ type: 'klinik' as FacilityFormValues['type'] }), 'create')
				.type
		).toBe('Tipe fasilitas tidak valid (pilih: rumah_sakit, puskesmas).');
	});

	it('requires the short code, which is the uniqueness key', () => {
		expect(validateFacilityForm(validForm({ short_code: '' }), 'create').short_code).toBe(
			'Kode singkat wajib diisi.'
		);
	});
});

/**
 * The asymmetry that matters most.
 *
 * Update is a PATCH, and the PATCH handler only enforces the fields it actually
 * receives. Demanding the create set on an update would block a rename — the
 * single most common edit — behind four untouched address fields the server
 * never asked for.
 */
describe('update demands only what the PATCH handler enforces', () => {
	it('accepts a one-field rename with everything else blank', () => {
		expect(validateFacilityForm({ ...blankFacility(), name: 'Klinik Baru' }, 'update')).toEqual({});
		expect(isFacilityFormValid({ ...blankFacility(), name: 'Klinik Baru' }, 'update')).toBe(true);
	});

	it('still refuses a blank name on update', () => {
		expect(validateFacilityForm(blankFacility(), 'update').name).toBe(
			'Nama fasilitas tidak boleh kosong.'
		);
	});

	it('does not require phone or short code on update', () => {
		const errors = validateFacilityForm({ ...blankFacility(), name: 'Klinik Baru' }, 'update');
		expect(errors.phone).toBeUndefined();
		expect(errors.short_code).toBeUndefined();
		expect(errors.address).toBeUndefined();
		expect(errors.type).toBeUndefined();
	});
});

describe('the phone rejects exactly the characters Go rejects', () => {
	it('refuses each forbidden character', () => {
		// One assertion per character on purpose. A combined string would pass
		// against a rule that caught only the first character, which would then
		// accept a payload the server refuses.
		for (const character of ['<', '>', '"', "'", '&']) {
			expect(
				hasForbiddenPhoneCharacter(`0812${character}3456789`),
				`${character} must be treated as forbidden`
			).toBe(true);
		}
	});

	it('names the phone field when a forbidden character is present', () => {
		expect(validateFacilityForm(validForm({ phone: '0812<script>' }), 'create').phone).toBe(
			'Nomor telepon mengandung karakter tidak valid.'
		);
	});

	it('accepts the ordinary characters a real number contains', () => {
		// The over-strict failure mode: a rule so broad it refuses valid numbers
		// and blocks every create.
		for (const phone of ['08123456789', '022-123456', '+62 812 3456', '(021) 555-123']) {
			expect(hasForbiddenPhoneCharacter(phone), `${phone} must be accepted`).toBe(false);
			expect(validateFacilityForm(validForm({ phone }), 'create').phone).toBeUndefined();
		}
	});

	it('rejects a script tag outright rather than stripping it', () => {
		// Sanitising instead of refusing would silently store a mangled number and
		// tell the operator it was saved.
		expect(validateFacilityForm(validForm({ phone: '<script>alert(1)</script>' }), 'create').phone)
			.toBe('Nomor telepon mengandung karakter tidak valid.');
	});

	it('exposes the character set the rule is built from, not a copy of it', () => {
		expect([...FORBIDDEN_PHONE_CHARACTERS].sort()).toEqual(['"', '&', "'", '<', '>'].sort());
	});
});

describe('bed counts cannot be negative or contradictory', () => {
	it('refuses a negative count', () => {
		expect(validateFacilityForm(validForm({ total_beds: '-1' }), 'create').total_beds).toBe(
			'Total tempat tidur tidak boleh negatif.'
		);
		expect(
			validateFacilityForm(validForm({ available_beds: '-2' }), 'create').available_beds
		).toBe('Tempat tidur tersedia tidak boleh negatif.');
	});

	it('refuses availability above the total', () => {
		expect(validateFacilityForm(validForm({ total_beds: '5', available_beds: '9' }), 'create')
			.available_beds).toBe('Tempat tidur tersedia tidak boleh melebihi total.');
	});

	it('allows a full house, which is a real state', () => {
		expect(
			validateFacilityForm(validForm({ total_beds: '5', available_beds: '5' }), 'create')
		).toEqual({});
	});
});
