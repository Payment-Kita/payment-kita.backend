-- Find duplicates and identify the kept ID vs duplicate IDs.
CREATE TEMP TABLE token_mapping AS
WITH ranked_tokens AS (
    SELECT id,
           chain_id,
           address,
           symbol,
           ROW_NUMBER() OVER (
               PARTITION BY chain_id, COALESCE(NULLIF(LOWER(address), ''), 'NATIVE_' || symbol)
               ORDER BY 
                   (logo_url IS NOT NULL AND logo_url <> '') DESC,
                   (max_amount IS NOT NULL) DESC,
                   created_at ASC
           ) as rn
    FROM tokens
),
kept_tokens AS (
    SELECT id, chain_id, COALESCE(NULLIF(LOWER(address), ''), 'NATIVE_' || symbol) as token_key
    FROM ranked_tokens
    WHERE rn = 1
),
duplicate_tokens AS (
    SELECT id, chain_id, COALESCE(NULLIF(LOWER(address), ''), 'NATIVE_' || symbol) as token_key
    FROM ranked_tokens
    WHERE rn > 1
)
SELECT d.id AS duplicate_id, k.id AS kept_id
FROM duplicate_tokens d
JOIN kept_tokens k ON d.chain_id = k.chain_id AND d.token_key = k.token_key;

-- Update references in payment_requests
UPDATE payment_requests pr
SET token_id = m.kept_id
FROM token_mapping m
WHERE pr.token_id = m.duplicate_id;

-- Update references in payments (source_token_id)
UPDATE payments p
SET source_token_id = m.kept_id
FROM token_mapping m
WHERE p.source_token_id = m.duplicate_id;

-- Update references in payments (dest_token_id)
UPDATE payments p
SET dest_token_id = m.kept_id
FROM token_mapping m
WHERE p.dest_token_id = m.duplicate_id;

-- Safely handle duplicate fee_configs to avoid unique constraint violations
DELETE FROM fee_configs fc
WHERE fc.token_id IN (SELECT duplicate_id FROM token_mapping)
  AND EXISTS (
      SELECT 1 FROM fee_configs fc2
      JOIN token_mapping m ON fc.token_id = m.duplicate_id
      WHERE fc2.chain_id = fc.chain_id AND fc2.token_id = m.kept_id
  );

-- Update references in fee_configs
UPDATE fee_configs fc
SET token_id = m.kept_id
FROM token_mapping m
WHERE fc.token_id = m.duplicate_id;

-- Delete the duplicate tokens from the main tokens table
DELETE FROM tokens
WHERE id IN (SELECT duplicate_id FROM token_mapping);

-- Drop mapping table
DROP TABLE token_mapping;

-- Add unique index to prevent future duplicates for non-native tokens (address-based)
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_tokens_chain_id_address 
ON tokens (chain_id, LOWER(address)) 
WHERE address IS NOT NULL AND address <> '';

-- Add unique index to prevent future duplicates for native tokens (symbol-based)
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_tokens_chain_id_native_symbol 
ON tokens (chain_id, symbol) 
WHERE address IS NULL OR address = '';
