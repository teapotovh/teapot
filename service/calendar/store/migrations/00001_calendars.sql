-- +brant Up
CREATE TABLE calendars (
  path TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT
);

-- +brant Down
DELETE TABLE calendars;
