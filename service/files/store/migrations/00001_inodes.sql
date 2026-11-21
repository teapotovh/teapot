-- +brant Up
CREATE TABLE inodes (
  uid BIGINT NOT NULL,
  node_id BIGINT NOT NULL,
  revision BIGINT NOT NULL,
  parent_id BIGINT NOT NULL,
  name TEXT NOT NULL,
  kind SMALLINT NOT NULL,
  ref TEXT NOT NULL,
  hash TEXT NOT NULL,
  size BIGINT NOT NULL,
  mtime TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (uid, node_id)
);

CREATE INDEX inodes_children_idx ON inodes (uid, parent_id, name);

CREATE TABLE revision (
  id INT PRIMARY KEY CHECK (id = 1),
  value BIGINT NOT NULL
);

INSERT INTO revision (id, value) VALUES (1, 0);
