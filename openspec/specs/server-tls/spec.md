## Purpose

Allow the emb server to accept TLS-encrypted connections, removing the need for a
separate proxy (stunnel, Envoy, nginx) when deploying across network boundaries.

## Requirements

### Requirement: Server supports TLS

The emb server SHALL support TLS encryption for incoming connections when configured
with both a certificate and key file.

#### Scenario: TLS enabled via config

- **WHEN** the config file has `tls_cert` and `tls_key` pointing to valid PEM files
- **THEN** the server SHALL start with TLS encryption
- **AND** the log SHALL indicate TLS is active

#### Scenario: TLS enabled via flags

- **WHEN** the server is started with `-tls-cert cert.pem -tls-key key.pem`
- **THEN** the server SHALL start with TLS encryption
- **AND** the log SHALL indicate TLS is active

#### Scenario: TLS client connection

- **WHEN** a Redis client connects with TLS to a TLS-enabled server
- **THEN** the server SHALL accept and handle RESP commands normally

#### Scenario: Plain TCP fallback

- **WHEN** neither `tls_cert` nor `tls_key` is configured
- **THEN** the server SHALL listen on plain TCP
- **AND** behavior SHALL be identical to current behavior

#### Scenario: Missing key file

- **WHEN** `tls_cert` is set but `tls_key` is not (or vice versa)
- **THEN** the server SHALL fail to start with an error message

#### Scenario: Invalid cert or key path

- **WHEN** `tls_cert` or `tls_key` points to a non-existent or unreadable file
- **THEN** the server SHALL fail to start with an error message
