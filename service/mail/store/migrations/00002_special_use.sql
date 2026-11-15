-- +brant Up
ALTER TABLE mailboxes ADD COLUMN special_use TEXT[] NOT NULL DEFAULT '{}';

-- +brant Down
ALTER TABLE mailboxes DROP COLUMN special_use;
