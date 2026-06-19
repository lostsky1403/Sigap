// Shared types for Sigap frontend
// Keep flat and minimal to avoid deep import trees.

export type Facility = {
	id: string;
	name: string;
	type: 'rumah_sakit' | 'puskesmas';
	kecamatan: string;
	kabupatenKota: string;
	totalBeds: number;
	availableBeds: number;
	lastUpdated: string;
	shortCode: string;
	lat?: number;
	lon?: number;
};

export type QueueTicket = {
	nomorAntrean: string;
	processingTime: string;
	facilityName: string;
	phone: string;
};

/** Response shape from POST /api/v1/queues/generate */
export type QueueApiResponse = {
	success?: boolean;
	data?: {
		formatted_number?: string;
		FormattedNumber?: string;
		ticket_id?: string;
		processing_time?: string;
		ProcessingTime?: string;
		[ key: string ]: unknown;
	};
	error?: string;
	[ key: string ]: unknown;
};

/** Response shape from GET /api/v1/facilities/nearby */
export type NearbyApiResponse = {
	success?: boolean;
	data?: Facility[];
	error?: string;
	[ key: string ]: unknown;
};

export type MedicalRecord = {
	facility_name: string;
	formatted_number: string;
	visit_time: string;
	signature: string;
	[ key: string ]: unknown;
};
