-- 000052_add_merchant_profile_fields.down.sql
-- Drop business information fields. Enum values addition cannot be easily partially reverted without recreating the entire type.

ALTER TABLE merchants DROP COLUMN IF EXISTS business_website;
ALTER TABLE merchants DROP COLUMN IF EXISTS business_description;
