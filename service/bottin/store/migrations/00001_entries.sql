-- +brant Up
CREATE TABLE entries (
  dn TEXT PRIMARY KEY,
  attributes JSONB NOT NULL
);

-- +brant Down
DELETE TABLE entries;
