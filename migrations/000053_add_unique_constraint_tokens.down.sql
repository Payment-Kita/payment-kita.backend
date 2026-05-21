-- Remove unique constraint
ALTER TABLE tokens DROP CONSTRAINT IF EXISTS tokens_address_chain_unique;

-- Drop the index
DROP INDEX IF EXISTS idx_tokens_address_chain;
