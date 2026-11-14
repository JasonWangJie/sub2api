-- Independent display marker: maintenance does not change group activation.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS is_maintenance BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN groups.is_maintenance IS 'Whether the group is marked as under maintenance';
