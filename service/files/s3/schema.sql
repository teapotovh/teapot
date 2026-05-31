CREATE TABLE inodes (
  id BIGSERIAL PRIMARY KEY,
  parent_id BIGINT REFERENCES inodes(id),
  name TEXT NOT NULL,

  size bigint NOT NULL,
  modified timestamp NOT NULL,
  mode bigint NOT NULL,

  UNIQUE(parent_id, name)
);
