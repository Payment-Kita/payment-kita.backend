-- Migration Down: Remove TokenSwapper smart contract for Base mainnet and restore original index

DELETE FROM smart_contracts
WHERE address = '0x8B6c7770D4B8AaD2d600e0cf5df3Eea5Bc0EB0fe'
  AND type = 'TOKEN_SWAPPER'
  AND chain_id = (SELECT id FROM chains WHERE chain_id = '8453' LIMIT 1);

DROP INDEX IF EXISTS idx_chain_address;

CREATE UNIQUE INDEX idx_chain_address ON smart_contracts (address) WHERE deleted_at IS NULL;
