-- +brant Up
CREATE TABLE mailboxes (
  id UUID PRIMARY KEY,
  owner TEXT NOT NULL,
  name TEXT NOT NULL,
  uid_validity BIGINT NOT NULL,
  uid_next BIGINT NOT NULL DEFAULT 1,
  highest_modseq BIGINT NOT NULL DEFAULT 0,
  change_seq BIGINT NOT NULL DEFAULT 0,
  subscribed BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX mailboxes_owner_name_idx
  ON mailboxes (owner, name)
  WHERE deleted_at IS NULL;

CREATE TABLE blobs (
  sha256 BYTEA PRIMARY KEY,
  size BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE messages (
  mailbox_id UUID NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
  uid BIGINT NOT NULL,
  blob_sha256 BYTEA NOT NULL REFERENCES blobs(sha256),
  flags TEXT[] NOT NULL DEFAULT '{}',
  internal_date TIMESTAMPTZ NOT NULL,
  size BIGINT NOT NULL,
  modseq BIGINT NOT NULL,
  change_seq BIGINT NOT NULL,
  expunged_at TIMESTAMPTZ,
  PRIMARY KEY (mailbox_id, uid)
);

CREATE INDEX messages_alive_idx
  ON messages (mailbox_id, uid)
  WHERE expunged_at IS NULL;

CREATE INDEX messages_change_idx
  ON messages (mailbox_id, change_seq);

-- +brant Down
DROP TABLE messages;
DROP TABLE blobs;
DROP TABLE mailboxes;
