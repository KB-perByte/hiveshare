-- Simplify membership to two roles: all (read/write/invite) and view (read-only).
-- Map legacy roles: owner/member → all, viewer → view.
-- Creator remains identifiable via hiveshares.owner_id.
--
-- IMPORTANT: drop the old CHECK before updating values.

ALTER TABLE hiveshare_members DROP CONSTRAINT IF EXISTS valid_role;

UPDATE hiveshare_members SET role = 'all' WHERE role IN ('owner', 'member', 'all');
UPDATE hiveshare_members SET role = 'view' WHERE role IN ('viewer', 'view');

-- Any unexpected leftover roles become all
UPDATE hiveshare_members SET role = 'all' WHERE role NOT IN ('all', 'view');

UPDATE invitations SET role = 'all' WHERE role IN ('owner', 'member') OR role IS NULL OR role = '' OR role = 'all';
UPDATE invitations SET role = 'view' WHERE role IN ('viewer', 'view');
UPDATE invitations SET role = 'all' WHERE role NOT IN ('all', 'view');

ALTER TABLE hiveshare_members
    ALTER COLUMN role SET DEFAULT 'all';

ALTER TABLE hiveshare_members
    ADD CONSTRAINT valid_role CHECK (role IN ('all', 'view'));
