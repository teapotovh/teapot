-- +brant Up
ALTER TABLE entries ALTER COLUMN dn TYPE TEXT COLLATE "C";

-- +brant Down
ALTER TABLE entries ALTER COLUMN dn TYPE TEXT COLLATE "default";
