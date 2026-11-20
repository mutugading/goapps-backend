-- [T033g1] pik_response dienkripsi aplikasi (pgp_sym_encrypt_bytea, kunci PAYROLL_PII_KEY): respons bisa memuat PII
-- hasil RevealPII (review PR #283). Kunci tidak ada di DB, jadi baris lama (plaintext) tidak bisa dienkripsi di sini:
-- respons tersimpan dibuang. Baris DONE ikut dihapus supaya replay tidak mengembalikan respons kosong; dampaknya
-- hanya pengulangan request dengan Idempotency-Key yang sama dalam 24 jam di sekitar deploy (sebelum go-live).
DELETE FROM payroll.pyr_idempotency_key WHERE pik_status = 'DONE' OR pik_response IS NOT NULL;
