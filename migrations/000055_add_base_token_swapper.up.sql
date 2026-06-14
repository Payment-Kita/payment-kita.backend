-- Migration: Drop global address unique constraint and recreate it as unique per chain
-- Then add TokenSwapper smart contract for Base mainnet

-- 1. Drop the old unique index
DROP INDEX IF EXISTS idx_chain_address;

-- 2. Recreate unique index on (chain_id, address) instead of (address) globally
CREATE UNIQUE INDEX idx_chain_address ON smart_contracts (chain_id, address) WHERE deleted_at IS NULL;

-- 3. Now insert the TokenSwapper for Base mainnet safely
INSERT INTO smart_contracts (
    id,
    name,
    chain_id,
    address,
    abi,
    type,
    version,
    deployer_address,
    is_active,
    metadata,
    created_at,
    updated_at
)
SELECT
    uuid_generate_v7(),
    'TokenSwapper',
    c.id,
    '0x8B6c7770D4B8AaD2d600e0cf5df3Eea5Bc0EB0fe',
    (SELECT abi FROM smart_contracts WHERE type = 'TOKEN_SWAPPER' AND is_active = true LIMIT 1),
    'TOKEN_SWAPPER',
    '2.1.0',
    '',
    TRUE,
    '{"source": "CHAIN_BASE.md", "sync_reason": "add_base_token_swapper"}'::jsonb,
    NOW(),
    NOW()
FROM chains c
WHERE c.deleted_at IS NULL
  AND c.chain_id = '8453'
  AND NOT EXISTS (
      SELECT 1 FROM smart_contracts
      WHERE chain_id = c.id
        AND type = 'TOKEN_SWAPPER'
        AND address = '0x8B6c7770D4B8AaD2d600e0cf5df3Eea5Bc0EB0fe'
        AND deleted_at IS NULL
  );
