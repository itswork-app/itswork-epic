-- PR-NEXUS-INTELLIGENCE: Intelligence Schema Extension
ALTER TABLE token_analysis ADD COLUMN IF NOT EXISTS reason TEXT;
ALTER TABLE token_analysis ADD COLUMN IF NOT EXISTS creator_reputation VARCHAR(50);
ALTER TABLE token_analysis ADD COLUMN IF NOT EXISTS insider_risk VARCHAR(50);
