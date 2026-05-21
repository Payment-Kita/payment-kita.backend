-- 000052_add_merchant_profile_fields.up.sql
-- Add business information fields to merchants table and expand merchant types.

-- Adding new enum values (Note: This may fail in some transaction-wrapped migration runners)
-- If this fails, the migration runner might need to be configured to run without transactions for this file.
-- PG 13+ supports IF NOT EXISTS for ADD VALUE, but we use standard for broader compatibility.
ALTER TYPE merchant_type_enum ADD VALUE 'SERVICES';
ALTER TYPE merchant_type_enum ADD VALUE 'DIGITAL';
ALTER TYPE merchant_type_enum ADD VALUE 'OTHER';

-- Add columns for website and description
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS business_website VARCHAR(255);
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS business_description TEXT;
