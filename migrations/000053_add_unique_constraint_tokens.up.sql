-- Add unique constraint on contract address + chain_id to prevent duplicates
-- First, clean up duplicates by keeping the most recently updated record
-- Only delete duplicates that are NOT referenced by other tables

-- Create a temporary table to identify duplicates
CREATE TEMP TABLE duplicate_tokens AS
SELECT 
    address,
    chain_id,
    id,
    ROW_NUMBER() OVER (PARTITION BY address, chain_id ORDER BY updated_at DESC, id DESC) as rn
FROM tokens
WHERE deleted_at IS NULL;

-- Create a mapping table for all duplicates (which token to keep vs delete)
CREATE TEMP TABLE token_mapping AS
SELECT 
    keep.id as keep_id,
    del.id as del_id,
    keep.address,
    keep.chain_id
FROM duplicate_tokens keep
JOIN duplicate_tokens del ON keep.address = del.address AND keep.chain_id = del.chain_id
WHERE keep.rn = 1 AND del.rn > 1;

-- Update all tables that reference tokens to point to the kept token
-- payment_requests
UPDATE payment_requests pr
SET token_id = tm.keep_id
FROM token_mapping tm
WHERE pr.token_id = tm.del_id;

-- payment_transaction_details (token_id)
UPDATE payment_transaction_details ptd
SET token_id = tm.keep_id
FROM token_mapping tm
WHERE ptd.token_id = tm.del_id;

-- swap_routes (source_token_id, dest_token_id)
UPDATE swap_routes sr
SET source_token_id = tm.keep_id
FROM token_mapping tm
WHERE sr.source_token_id = tm.del_id;

UPDATE swap_routes sr
SET dest_token_id = tm.keep_id
FROM token_mapping tm
WHERE sr.dest_token_id = tm.del_id;

-- payment_routes (source_token_id, dest_token_id)
UPDATE payment_routes pr
SET source_token_id = tm.keep_id
FROM token_mapping tm
WHERE pr.source_token_id = tm.del_id;

UPDATE payment_routes pr
SET dest_token_id = tm.keep_id
FROM token_mapping tm
WHERE pr.dest_token_id = tm.del_id;

-- Now delete all duplicates
DELETE FROM tokens
WHERE id IN (
    SELECT id FROM duplicate_tokens WHERE rn > 1
);

-- Add unique constraint to prevent future duplicates
ALTER TABLE tokens 
ADD CONSTRAINT tokens_address_chain_unique UNIQUE (address, chain_id);

-- Add index for better query performance
CREATE INDEX IF NOT EXISTS idx_tokens_address_chain ON tokens(address, chain_id);

-- Drop temporary tables
DROP TABLE IF EXISTS duplicate_tokens;
DROP TABLE IF EXISTS token_mapping;
