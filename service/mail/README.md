# Mail service

The mail service stores mail in PostgreSQL and S3. It provides an IMAP server
and an LMTP server. PostgreSQL stores mailbox metadata, flags, UIDs, and message
bodies' checksums. S3 stores the raw RFC 822 message bytes.

## Components

- `store`: PostgreSQL metadata and S3 blob storage.
- `imap`: IMAP server built on `emersion/go-imap` v2.
- `lmtp`: LMTP server built on `emersion/go-smtp`.
- `recipient`: LDAP recipient resolution for LMTP.

## Local development

```sh
maild \
  --mail-store-type online \
  --mail-store-psql-url postgresql://user:password@localhost:5432/mydb \
  --mail-store-s3-url http://root:rootroot@localhost:9000 \
  --mail-store-s3-bucket mail \
  --s3cache-path /tmp/mail-cache \
  --mail-imap-addr 127.0.0.1:1143 \
  --mail-imap-insecure-auth \
  --mail-lmtp-addr 127.0.0.1:1024 \
  --mail-lmtp-domain mail.example.org
```

LDAP flags are the same as the other services. The IMAP login uses LDAP bind.
The LMTP server resolves the final recipient address through the LDAP `mail`
attribute.

## Tests

All tests use an in-memory store and run with Bazel:

```sh
bazel test //service/mail/...
```

The end-to-end test delivers a message over LMTP, then reads it and searches
it over IMAP.

The PostgreSQL store sends `NOTIFY` on mailbox changes. IMAP sessions listen for
these notifications and also poll at a slower fallback interval. Set the
fallback with `--mail-imap-poll-interval` (default `10s`).

## Postfix LMTP transport

Example Postfix transport:

```text
mail.example.org  lmtp:unix:/run/teapot/lmtp.sock
```

The service does not configure Postfix. Postfix must handle aliases and relay
decisions. The LMTP service receives only final local recipients.
