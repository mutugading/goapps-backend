INSERT INTO cost_product_type (cpt_type_code, cpt_type_name) VALUES
  ('ACY',  'Air Covered Yarn'),
  ('ATT',  'Attached Type Yarn'),
  ('HOY',  'High Oriented Yarn'),
  ('IDY',  'Intermingled Draw Yarn'),
  ('ITY',  'Interlaced Textured Yarn'),
  ('NTY',  'Nylon Textured Yarn'),
  ('OTH',  'Other'),
  ('PTS',  'Polyester Textured Slub'),
  ('TCH',  'Twisted Compound Hybrid'),
  ('TCS',  'Twisted Compound Slub'),
  ('TFY',  'Twisted Filament Yarn'),
  ('TPM',  'Twisted Polyester Multifilament'),
  ('TPS',  'Twisted Polyester Slub'),
  ('TPY',  'Twisted Polyester Yarn'),
  ('TTM',  'Twisted Textured Multifilament'),
  ('TTS',  'Twisted Textured Slub')
ON CONFLICT (cpt_type_code) DO UPDATE
  SET cpt_type_name = EXCLUDED.cpt_type_name;
