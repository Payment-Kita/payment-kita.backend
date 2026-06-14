-- Migration: Deactivate duplicate active smart contracts
-- Group by (chain_id, type, name) and set is_active = false for duplicates (rn > 1) ordered by version DESC, updated_at DESC, created_at DESC.

WITH duplicate_contracts AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY chain_id, type, name
               ORDER BY 
                   -- Sort by version (parsed as semver array if possible, otherwise string fallback)
                   CASE 
                       WHEN version ~ '^[0-9]+\.[0-9]+\.[0-9]+$' THEN string_to_array(version, '.')::int[]
                       ELSE ARRAY[0,0,0]
                   END DESC,
                   updated_at DESC,
                   created_at DESC,
                   id DESC
           ) as rn
    FROM smart_contracts
    WHERE is_active = true 
      AND type NOT IN ('POOL', 'DEX_POOL') 
      AND deleted_at IS NULL
)
UPDATE smart_contracts
SET is_active = false,
    updated_at = NOW()
WHERE id IN (
    SELECT id 
    FROM duplicate_contracts 
    WHERE rn > 1
);
