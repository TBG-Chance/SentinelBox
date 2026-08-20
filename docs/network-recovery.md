# Routed Network Recovery

Use these steps from the SentinelBox appliance's physical console. Do not rely
on a remote session that traverses the firewall or interface being changed.

## Before applying

1. Label the physical WAN and LAN cables and record their predictable Debian
   interface names.
2. Keep a copy of the previously working network, dnsmasq, sysctl, and nftables
   configuration outside `/etc`.
3. Confirm that the current configuration does not already use the table names
   `sentinelbox_filter` or `sentinelbox_nat` for another purpose.
4. Render and review the proposed files, then run `validate` and `check`.
5. Keep this recovery document open at the physical console.

## Remove the SentinelBox firewall boundary

The preferred recovery command discovers and deletes only SentinelBox-owned
tables:

```text
sudo sentinelbox-network remove-owned \
  --config /etc/sentinelbox/sentinelbox.toml \
  --confirm-remove-owned-tables
```

If the utility is unavailable, inspect first and then remove those exact tables
with nftables:

```text
sudo nft --json list ruleset
sudo nft delete table inet sentinelbox_filter
sudo nft delete table ip sentinelbox_nat
```

A delete command may report that a table does not exist. Never substitute
`flush ruleset`: that would remove firewall state owned by Debian or other
software.

## Restore base networking

If routing, DHCP, or interface addressing is the failure, keep the firewall
tables removed and restore the known-good files for the active Debian network
manager and dnsmasq from the console. Remove or move aside only the reviewed
SentinelBox files, reload sysctl settings, and restart the affected services.
Do not disable an alternate network manager until the restored configuration
has been confirmed.

If forwarding must be stopped immediately while configuration is restored:

```text
sudo sysctl -w net.ipv4.ip_forward=0
sudo sysctl -w net.ipv6.conf.all.forwarding=0
```

## Verify recovery

- confirm the console remains usable;
- inspect interface addresses and routes;
- confirm SentinelBox-owned tables are absent;
- confirm unrelated nftables tables remain present;
- verify the restored network manager is active;
- verify the DHCP service is stopped or serving only the intended LAN;
- document the failure and recovery before another apply attempt.

Recovery removes the SentinelBox routed firewall policy. Keep the appliance off
an untrusted inline path until the known-good configuration is restored and
verified.
