# SentinelBox

SentinelBox is a local-first, explainable network-defense appliance for small
and medium businesses. It is designed for a Dell OptiPlex 5000 Micro (D15U)
running Debian Linux with one built-in Ethernet interface and one USB 3.x
Ethernet adapter.

## Current status

Milestone 1 is complete. The Milestone 2 routed-network software is implemented
and ready for isolated Debian appliance validation. The project provides:

- strict TOML configuration with safe startup validation;
- structured JSON or text logging through Go's standard library;
- embedded, checksummed, forward-only SQLite migrations;
- SQLite WAL mode, foreign-key enforcement, bounded connections, and a busy
  timeout;
- a loopback-only health/readiness API;
- signal-aware graceful shutdown; and
- unit and integration tests that require neither root nor live firewall
  access;
- explicit WAN/LAN topology validation;
- reviewed `systemd-networkd`, dnsmasq, sysctl, and nftables artifact
  generation; and
- a separate root-only utility that dry-runs, applies, verifies, and removes
  only SentinelBox-owned nftables tables.

SentinelBox does **not yet ingest Suricata events, inventory devices, calculate
risk, or perform device-specific response**. The routed foundation is not ready
for production or inline placement until the physical Milestone 2 acceptance
gate passes.

## Architecture boundary

Future behavior must preserve:

```text
Security Event -> Signal -> Correlation -> Risk -> Policy -> Response
```

Suricata will detect. SentinelBox will interpret, correlate, score, explain,
decide, and request reversible enforcement. Detection ingestion will never
manipulate nftables directly.

See [docs/architecture.md](docs/architecture.md) for the current boundaries and
the routed-mode limitation.

## Requirements

- Go 1.26 or later for development
- Debian Linux for the target appliance
- No root privileges for the service, renderer, or test suite
- Root on the dedicated Debian appliance for live nftables operations

Runtime dependencies are embedded in the Go binary. SQLite does not require a
separate database service.

## Validate the foundation

```text
go test ./...
go test -race ./...
go vet ./...
```

## Prepare routed networking

Use `config/sentinelbox.routed.example.toml` as the appliance template. The
network utility can render the complete file bundle on any development host
without changing networking:

```text
go run ./cmd/sentinelbox-network render \
  --config /absolute/path/to/sentinelbox.routed.toml \
  --output /absolute/path/to/staging
```

Review [docs/milestone-2.md](docs/milestone-2.md) before appliance installation
and keep [docs/network-recovery.md](docs/network-recovery.md) available at its
physical console. Live firewall operations are explicit, Linux-only, and never
flush unrelated nftables state.

## Run on a Debian development system

1. Copy `config/sentinelbox.example.toml` to a local file.
2. Set `database.path` to an absolute path writable by the current user.
3. Keep `server.listen` on loopback and keep Suricata and automatic response
   disabled. Use the foundation example with networking disabled unless this is
   the isolated Milestone 2 appliance.
4. Start SentinelBox:

```text
go run ./cmd/sentinelbox -config /absolute/path/to/sentinelbox.local.toml
```

5. Check health:

```text
curl --fail http://127.0.0.1:8080/api/v1/health
```

The response includes application version, source revision, build time,
database readiness, schema version, UTC time, and a request ID.

## Release build metadata

Release automation can replace the default development metadata:

```text
go build -trimpath -ldflags "-s -w \
  -X github.com/TBG-Chance/SentinelBox/internal/buildinfo.Version=0.1.0 \
  -X github.com/TBG-Chance/SentinelBox/internal/buildinfo.Revision=<revision> \
  -X github.com/TBG-Chance/SentinelBox/internal/buildinfo.BuildTime=<utc-time>" \
  -o bin/sentinelbox ./cmd/sentinelbox
```

## Configuration safety

SentinelBox refuses to start when configuration contains unknown fields,
non-absolute data paths, unordered risk thresholds, WAN management exposure,
IPv6 forwarding without policy parity, or automatic response before the
verified enforcer milestone.

The example configuration is a template, not a secret store. Do not place
passwords, session keys, certificates, or tokens in it.

## Database safety

The database and migration files are checksummed. SentinelBox refuses startup
if an applied migration was changed or the database was created by an unknown
newer schema. Production code applies forward migrations only.

SQLite WAL creates `-wal` and `-shm` files next to the database. Backups must
use an approved online-backup or checkpoint procedure; copying only the main
database file while the application is running is not sufficient.

## Project scope

The prototype remains intentionally SMB-focused and does not aim to become an
enterprise SIEM, EDR platform, general firewall replacement, or cloud-managed
service. Each development milestone must be completed and verified before the
next milestone begins.
