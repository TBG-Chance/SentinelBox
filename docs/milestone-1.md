# Milestone 1 - Project Foundation

## Scope delivered

- Go module and repository structure
- Strict versioned TOML configuration
- Safe foundation defaults and validation
- Structured standard-library logging
- SQLite initialization and connection policy
- Embedded forward-only migration runner
- Checksummed migration history
- Health/readiness API
- Request identifiers and basic response security headers
- Graceful HTTP shutdown and database close
- Unit and integration tests
- Foundation, security, and architecture documentation

## Explicitly deferred

- Routed networking and DHCP
- Suricata installation or EVE ingestion
- Device discovery and inventory
- Events, signals, correlation, risk, policy, or response models
- nftables mutation
- Administrator authentication and HTTPS
- Dashboard

## Acceptance evidence

Run from the repository root:

```text
go test ./...
go test -race ./...
go vet ./...
```

Manual health validation requires a configuration with an absolute writable
database path. The API must report `healthy`, database `ready`, and schema
version `1` on loopback. Cancellation or an operating-system termination signal
must drain HTTP requests and close SQLite cleanly.
