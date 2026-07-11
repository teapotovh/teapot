-- +brant Up
CREATE TABLE objects (
  path TEXT PRIMARY KEY,
  mod_time TIMESTAMPTZ NOT NULL,
  data BYTEA NOT NULL
);

-- +brant Down
DROP TABLE objects;
