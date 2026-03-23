-- PR-13: Solana Pay Integration & Data Persistence (DOWN)
DROP TABLE IF EXISTS payments;
ALTER TABLE token_analysis DROP COLUMN IF EXISTS reason;
