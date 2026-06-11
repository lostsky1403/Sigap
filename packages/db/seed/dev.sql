-- dev.sql seed for Sigap (realistic but fictional data for local dev)
-- Run after 0001_init.sql

INSERT INTO facilities (name, type, address, kecamatan, kabupaten_kota, provinsi, phone, total_beds, available_beds, short_code, is_active) VALUES
('RSUD Kota Sehat', 'rumah_sakit', 'Jl. Kesehatan No. 1', 'Sukamaju', 'Kota Bandung', 'Jawa Barat', '022-123456', 180, 42, 'RSK', true),
('Puskesmas Sukajaya', 'puskesmas', 'Jl. Melati No. 7', 'Sukajaya', 'Kab. Bandung', 'Jawa Barat', '022-654321', 28, 19, 'PKM', true),
('RS Mitra Sehat', 'rumah_sakit', 'Jl. Sudirman No. 45', 'Menteng', 'Jakarta Pusat', 'DKI Jakarta', '021-987654', 95, 11, 'RSM', true),
('Puskesmas Melati Indah', 'puskesmas', 'Jl. Anggrek No. 12', 'Cilandak', 'Jakarta Selatan', 'DKI Jakarta', '021-555123', 35, 27, 'PMI', true),
('RSUD Sejahtera', 'rumah_sakit', 'Jl. Merdeka No. 88', 'Cibadak', 'Kab. Sukabumi', 'Jawa Barat', '0266-212121', 120, 68, 'RSJ', true),
('Puskesmas Harapan Baru', 'puskesmas', 'Jl. Raya No. 3', 'Parung', 'Kab. Bogor', 'Jawa Barat', '0251-876543', 22, 5, 'PHB', true);

-- Note: patient and queue seed data added during runtime tests or later seed script
