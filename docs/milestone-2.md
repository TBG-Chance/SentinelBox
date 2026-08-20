# Milestone 2 - Routed Network Foundation

## Status

The software implementation is complete and locally verified. Final acceptance
requires a dedicated Debian appliance, both physical Ethernet adapters, an
isolated test switch, and a test WAN. No workstation networking was changed
during development.

## Scope delivered

- explicit WAN and LAN roles; SentinelBox never infers a role from interface
  enumeration order;
- live validation that both roles exist, are distinct, are administratively
  up, are not loopback, and expose hardware addresses;
- deterministic staging of `systemd-networkd`, dnsmasq, sysctl, and nftables
  configuration;
- a WAN DHCPv4 client and a static IPv4 LAN gateway;
- LAN-only DHCP and DNS service configuration;
- IPv4 forwarding and LAN-to-WAN masquerading;
- default-drop input and forwarding chains, with only the minimum routed-mode
  allowances;
- IPv6 forwarding disabled until equivalent IPv6 policy is implemented;
- dry-run, explicit-confirmation apply, post-apply verification, and recovery
  operations for SentinelBox-owned nftables tables;
- unit tests that exercise generation, topology failures, command construction,
  verification failure, and owned-table cleanup without requiring root.

The long-running `sentinelbox` service remains unprivileged and does not alter
networking at startup. `sentinelbox-network` is a separate installation and
recovery utility. It invokes the configured absolute `nft` executable directly
with fixed arguments and rules on standard input; it does not invoke a shell.

## Owned firewall boundary

SentinelBox owns exactly these tables:

| Family | Table | Purpose |
| --- | --- | --- |
| `inet` | `sentinelbox_filter` | Host input and routed forwarding policy |
| `ip` | `sentinelbox_nat` | IPv4 LAN-to-WAN masquerading |

The utility discovers whether those tables exist, builds one replacement
batch, asks nftables to check the batch, applies it atomically, and verifies the
expected tables and sets through nftables JSON output. It never emits `flush
ruleset` and never deletes an unrelated table. A failed post-apply verification
causes best-effort removal of both owned tables.

The following updateable IPv4 sets are reserved for later response milestones:

- `sbox_watch`
- `sbox_restricted`
- `sbox_contained`
- `sbox_trusted`

They do not enforce device-specific policy in Milestone 2.

## Render and inspect on any development host

Copy `config/sentinelbox.routed.example.toml`, replace both interface names,
and render to a new absolute staging directory:

```text
go run ./cmd/sentinelbox-network render \
  --config /absolute/path/to/sentinelbox.routed.toml \
  --output /absolute/path/to/staging
```

The command only writes below the selected staging directory. Review every
generated file before installing it on the appliance. The relative paths show
their intended Debian locations:

```text
etc/systemd/network/10-sentinelbox-wan.network
etc/systemd/network/20-sentinelbox-lan.network
etc/dnsmasq.d/sentinelbox.conf
etc/sysctl.d/90-sentinelbox-forwarding.conf
etc/nftables.d/sentinelbox.nft
```

Installation into `/etc`, switching the active Debian network manager, and
restarting network services are intentionally operator-controlled steps. They
can disconnect the machine and must be performed from its physical console
with a reviewed rollback plan.

## Appliance checks

Before a firewall change, confirm the configured physical roles:

```text
sudo sentinelbox-network validate \
  --config /etc/sentinelbox/sentinelbox.toml
```

Ask nftables to validate the complete replacement without applying it:

```text
sudo sentinelbox-network check \
  --config /etc/sentinelbox/sentinelbox.toml
```

Apply only after console access and recovery steps are ready:

```text
sudo sentinelbox-network apply \
  --config /etc/sentinelbox/sentinelbox.toml \
  --confirm-routed-network-change
```

The apply operation changes only the two owned nftables tables. The staged
networkd, dnsmasq, and sysctl files remain installation artifacts; the utility
does not copy them into `/etc` or restart services.

## Physical acceptance gate

Milestone 2 is accepted on hardware only after all of the following pass:

- the built-in Ethernet interface consistently retains the configured WAN
  role, and the USB 3.x Ethernet adapter consistently retains the LAN role;
- the WAN receives its expected DHCPv4 lease;
- the LAN gateway owns the configured address and no unintended IPv6 route is
  available;
- a downstream client receives an address, router, and DNS server from the
  configured DHCP pool;
- the client reaches the internet through IPv4 NAT and DNS;
- unsolicited WAN-to-LAN forwarding and WAN management access are denied;
- the loopback management endpoint is not exposed to either network;
- reboot, power-loss recovery, USB disconnect/reconnect, and a sustained
  24-hour traffic test do not swap roles or lose stable connectivity;
- measured routed throughput and latency are acceptable for the target SMB
  connection.

Record appliance model, BIOS version, Debian version, kernel, NIC chipsets,
adapter firmware, test duration, throughput, packet loss, and any driver resets
with the acceptance result.

## Containment limitation

This topology controls traffic that crosses SentinelBox. Two clients on the
same downstream unmanaged switch and IPv4 subnet can communicate directly,
without traversing the appliance. Milestone 2 therefore establishes a routed
security boundary but does not provide complete same-subnet lateral
containment. That requires client isolation, VLANs, or multiple routed zones.

## Explicitly deferred

- Suricata installation and event ingestion
- passive or active device discovery
- device inventory and identity correlation
- risk scoring, policy decisions, or automated response
- use of the reserved nftables sets for enforcement
- administrator authentication, HTTPS, and LAN management access
- dashboard and alerts

## Local acceptance evidence

Run from the repository root:

```text
go mod verify
go test ./...
go vet ./...
GOOS=linux GOARCH=amd64 go build ./cmd/sentinelbox
GOOS=linux GOARCH=amd64 go build ./cmd/sentinelbox-network
```

The Linux CI job also runs the race detector. Live network acceptance belongs
on an isolated appliance, not on a developer workstation or production LAN.

## Configuration references

- [Debian `systemd.network` manual](https://manpages.debian.org/trixie/systemd/systemd.network.5.en.html)
- [nftables command manual](https://netfilter.org/projects/nftables/manpage.html)
- [dnsmasq manual](https://thekelleys.org.uk/dnsmasq/docs/dnsmasq-man.html)
