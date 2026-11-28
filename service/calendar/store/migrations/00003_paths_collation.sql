-- +brant Up
ALTER TABLE calendars ALTER COLUMN path TYPE TEXT COLLATE "C";
ALTER TABLE object_refs ALTER COLUMN path TYPE TEXT COLLATE "C";

-- +brant Down
ALTER TABLE calendars ALTER COLUMN path TYPE TEXT COLLATE "default";
ALTER TABLE object_refs ALTER COLUMN path TYPE TEXT COLLATE "default";
