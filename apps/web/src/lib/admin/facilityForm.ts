/**
 * T-3B5-04: facility form validation, mirroring `validateCreateFacility` and
 * `validateUpdateFacility` in admin.go.
 *
 * The phone rule deserves a note, because it looks arbitrary and is not.
 *
 * Go rejects a phone containing any of `< > " ' &`:
 *
 *     if strings.ContainsAny(req.Phone, "<>\"'&")
 *
 * These are the characters that break out of an HTML attribute or a text node
 * in the surfaces this data is later rendered into. The API does not escape on
 * the way out because it is not its job to know the consumers — so the
 * restriction lives on input, at the point where the data enters the system,
 * where it is cheap and applies to every future consumer at once.
 *
 * So this is a real rule and the client enforces it, with the server's own
 * message. It is NOT a digit-only or length rule, because the API does not
 * impose one: Indonesian landlines and the `000-000000` placeholder the seed
 * uses both contain characters outside `[0-9]`, and inventing a stricter rule
 * here would reject data the server accepts.
 *
 * The bed rules are enforced on create in full, and on update only for the
 * fields actually present — matching the two Go functions, which is why
 * `validateFacilityForm` takes a mode.
 */

/** Mirrors validFacilityTypes. */
export const FACILITY_TYPES = ['rumah_sakit', 'puskesmas'] as const;
export type FacilityType = (typeof FACILITY_TYPES)[number];

/** The exact character set Go's ContainsAny rejects. */
export const FORBIDDEN_PHONE_CHARACTERS = '<>"\'' + '&';

export interface FacilityFormValues {
	name: string;
	type: FacilityType | '';
	address: string;
	kecamatan: string;
	kabupaten_kota: string;
	provinsi: string;
	phone: string;
	/** Kept as a string: a number input that is cleared reports "", not 0. */
	total_beds: string;
	available_beds: string;
	short_code: string;
}

export type FacilityFormErrors = Partial<Record<keyof FacilityFormValues, string>>;

export function blankFacility(): FacilityFormValues {
	return {
		name: '',
		type: '',
		address: '',
		kecamatan: '',
		kabupaten_kota: '',
		provinsi: '',
		phone: '',
		total_beds: '0',
		available_beds: '0',
		short_code: ''
	};
}

/** True when the value contains any character Go's ContainsAny rejects. */
export function hasForbiddenPhoneCharacter(value: string): boolean {
	for (const character of FORBIDDEN_PHONE_CHARACTERS) {
		if (value.includes(character)) return true;
	}
	return false;
}

function parseBedCount(value: string): number {
	const trimmed = value.trim();
	if (trimmed === '') return Number.NaN;
	const parsed = Number(trimmed);
	return Number.isInteger(parsed) ? parsed : Number.NaN;
}

/**
 * Validates the form.
 *
 * `mode: 'create'` applies the full set the POST handler applies, including the
 * required address fields. `mode: 'update'` applies only what the PATCH handler
 * applies, so an edit that changes one field is not told that six other fields
 * are required — the API does not require them either, and requiring them here
 * would block a legitimate partial update.
 */
export function validateFacilityForm(
	values: FacilityFormValues,
	mode: 'create' | 'update' = 'create'
): FacilityFormErrors {
	const errors: FacilityFormErrors = {};
	const isCreate = mode === 'create';

	if (values.name.trim() === '') {
		errors.name = isCreate
			? 'Nama fasilitas wajib diisi.'
			: 'Nama fasilitas tidak boleh kosong.';
	}

	if (values.type !== '' && !FACILITY_TYPES.includes(values.type as FacilityType)) {
		errors.type = 'Tipe fasilitas tidak valid (pilih: rumah_sakit, puskesmas).';
	} else if (isCreate && values.type === '') {
		errors.type = 'Tipe fasilitas tidak valid (pilih: rumah_sakit, puskesmas).';
	}

	if (isCreate) {
		if (
			values.address.trim() === '' ||
			values.kecamatan.trim() === '' ||
			values.kabupaten_kota.trim() === '' ||
			values.provinsi.trim() === ''
		) {
			// One message for the group, as the server does: naming which of the
			// four was missed would be a guess, and the operator can see all four.
			errors.address =
				'Alamat lengkap (jalan, kecamatan, kabupaten/kota, provinsi) wajib diisi.';
		}
	}

	const total = parseBedCount(values.total_beds);
	const available = parseBedCount(values.available_beds);

	if (isCreate || values.total_beds.trim() !== '') {
		if (Number.isNaN(total)) {
			errors.total_beds = 'Total tempat tidur tidak boleh negatif.';
		} else if (total < 0) {
			errors.total_beds = 'Total tempat tidur tidak boleh negatif.';
		}
	}

	if (isCreate || values.available_beds.trim() !== '') {
		if (Number.isNaN(available)) {
			errors.available_beds = 'Tempat tidur tersedia tidak boleh negatif.';
		} else if (available < 0) {
			errors.available_beds = 'Tempat tidur tersedia tidak boleh negatif.';
		}
	}

	// The cross-field rule only bites when BOTH counts are known, which mirrors
	// the Go condition (`req.TotalBeds != nil && req.AvailableBeds != nil`).
	if (!Number.isNaN(total) && !Number.isNaN(available) && available > total) {
		errors.available_beds = 'Tempat tidur tersedia tidak boleh melebihi total.';
	}

	if (isCreate && values.phone.trim() === '') {
		errors.phone = 'Nomor telepon wajib diisi.';
	} else if (values.phone !== '' && hasForbiddenPhoneCharacter(values.phone)) {
		errors.phone = 'Nomor telepon mengandung karakter tidak valid.';
	}

	if (isCreate && values.short_code.trim() === '') {
		errors.short_code = 'Kode singkat wajib diisi.';
	}

	return errors;
}

export function isFacilityFormValid(
	values: FacilityFormValues,
	mode: 'create' | 'update' = 'create'
): boolean {
	return Object.keys(validateFacilityForm(values, mode)).length === 0;
}

/**
 * Builds the create body.
 *
 * `available_beds` is included because the API's create request declares it and
 * the existing `FacilityInput` type omitted it — a gap that would have made
 * every created facility default its availability to whatever the column's
 * default is, which is not something the operator asked for.
 *
 * Numbers are converted here so the JSON carries ints, matching Go's decode.
 * An empty bed field becomes 0 rather than NaN, because `JSON.stringify(NaN)`
 * produces `null` and Go's `int` decode rejects null with a type error instead
 * of the validation message naming the real problem.
 */
export function toFacilityCreateRequest(values: FacilityFormValues): {
	name: string;
	type: FacilityType;
	address: string;
	kecamatan: string;
	kabupaten_kota: string;
	provinsi: string;
	phone: string;
	total_beds: number;
	available_beds: number;
	short_code: string;
} {
	return {
		name: values.name.trim(),
		type: values.type as FacilityType,
		address: values.address.trim(),
		kecamatan: values.kecamatan.trim(),
		kabupaten_kota: values.kabupaten_kota.trim(),
		provinsi: values.provinsi.trim(),
		phone: values.phone.trim(),
		total_beds: parseBedCount(values.total_beds) || 0,
		available_beds: parseBedCount(values.available_beds) || 0,
		short_code: values.short_code.trim()
	};
}

/**
 * Builds the update body from only the fields that changed.
 *
 * A partial update is legal on the server, and sending every field would be
 * wrong in a way that matters: a form that was seeded from a row the operator
 * only meant to rename would also write back the address, the bed counts, and
 * the short code. If the row changed underneath them in the meantime, their
 * edit would silently revert someone else's change. Sending only the diff makes
 * the update mean what it says.
 */
export function toFacilityUpdateRequest(
	values: FacilityFormValues,
	original: FacilityFormValues
): Record<string, string | number> {
	const next = toFacilityCreateRequest(values);
	const before = toFacilityCreateRequest(original);
	const body: Record<string, string | number> = {};

	for (const key of Object.keys(next) as (keyof typeof next)[]) {
		if (next[key] !== before[key]) {
			body[key] = next[key];
		}
	}
	return body;
}
