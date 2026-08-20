# Security Policy

SentinelBox is pre-release security infrastructure. It is not approved for
production use or deployment on an untrusted network.

## Reporting

Do not disclose suspected vulnerabilities through a public issue. Use the
repository owner's private security-reporting channel or security-advisory
workflow once the repository is published. Include reproduction steps, impact,
affected version, and any suggested mitigation. Do not include credentials,
customer data, packet payloads, or unrelated personal information.

## Current security boundary

Management remains bound to loopback and the service performs no packet
capture. Milestone 2 adds reviewed routed-network artifacts and a separate
root-only operator utility. That utility directly invokes an absolute `nft`
executable, owns exactly two named tables, checks changes before applying them,
verifies the result, and provides owned-table recovery. The service itself does
not receive firewall privileges. Authentication, TLS, packet inspection, and
device-specific enforcement remain gated milestones.

## Development requirements

- Use parameterized SQL.
- Never construct shell commands from user-controlled input.
- Keep the risk and policy engines independent from enforcement.
- Keep the control plane unprivileged.
- Treat every enforcement action as failed until it is verified.
- Keep containment reversible and auditable.
- Do not log secrets or unnecessary packet content.
- Run tests and dependency review before merging changes.
