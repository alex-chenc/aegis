-- Migration: V2.8 - Add display_name and file_md5 columns
-- Date: 2026-03-11

-- Add display_name column (copy from name for existing records)
ALTER TABLE templates ADD COLUMN IF NOT EXISTS display_name VARCHAR(255);
UPDATE templates SET display_name = name WHERE display_name IS NULL;
ALTER TABLE templates ALTER COLUMN display_name SET NOT NULL;

-- Add file_md5 column
ALTER TABLE templates ADD COLUMN IF NOT EXISTS file_md5 VARCHAR(32);

-- Create index on file_md5
CREATE INDEX IF NOT EXISTS idx_templates_md5 ON templates(file_md5);