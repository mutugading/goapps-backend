-- [T033g1] Baris ber-respons terenkripsi tidak terbaca oleh kode lama (yang mengharapkan plaintext): buang.
DELETE FROM payroll.pyr_idempotency_key WHERE pik_status = 'DONE' OR pik_response IS NOT NULL;
