-- +brant Up
CREATE TABLE calendars (
  path TEXT PRIMARY KEY,
  metadata JSONB NOT NULL
);

-- +brant Down
DELETE TABLE calendars;
