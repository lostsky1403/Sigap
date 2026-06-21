package notification

import "testing"

func TestContainsRawPhoneDigits_AcceptsSafeText(t *testing.T) {
	safe := []string{
		"",
		"Konfirmasi Janji Temu Sigap",
		"Kode check-in: ABC123",
		"Pada 2026-06-22 09:00",
		"Nomor antrian Anda: 0042",
		"Fasilitas: RSUD Kota Sehat",
		"Janji temu Anda berhasil dicatat.", // has "0"+"6"+"9" — but not 8 consecutive
	}
	for _, s := range safe {
		if ContainsRawPhoneDigits(s) {
			t.Errorf("ContainsRawPhoneDigits(%q) returned true for safe text", s)
		}
	}
}

func TestContainsRawPhoneDigits_RejectsPhoneLike(t *testing.T) {
	unsafe := []string{
		"Konfirmasi +6281234567890",
		"Hubungi 081234567890 untuk info",
		"Phone: 6281234567890",
		"Tel: 12345678",            // exactly 8 digits — should be caught
		"Nomor telp: 123456789012", // 12 digits
	}
	for _, s := range unsafe {
		if !ContainsRawPhoneDigits(s) {
			t.Errorf("ContainsRawPhoneDigits(%q) returned false for phone-like text", s)
		}
	}
}

func TestContainsRawPhoneDigits_AllowsShortDigitRuns(t *testing.T) {
	// 7 digits or fewer should NOT trigger (avoids false positives on
	// short numeric IDs like "Order #1234567").
	short := []string{
		"Order #1234567",        // 7 digits
		"Code: 1234",            // 4 digits
		"12-34-56",              // 6 digits with separators
		"Floors 1-2-3-4-5",      // 5 digits with separators
	}
	for _, s := range short {
		if ContainsRawPhoneDigits(s) {
			t.Errorf("ContainsRawPhoneDigits(%q) returned true for short digit run", s)
		}
	}
}
