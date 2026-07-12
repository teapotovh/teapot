-- +brant Up
CREATE TABLE object_refs (
  path TEXT PRIMARY KEY,
  mod_time TIMESTAMPTZ NOT NULL,
  ref UUID NOT NULL,
  etag TEXT NOT NULL
);

-- +brant Down
DROP TABLE object_refs;
