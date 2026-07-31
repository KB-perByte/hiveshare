# Security Policy

## Scope

This policy covers the HiveShare server, CLI, and MCP sidecar in this repository.

HiveShare is a self-hosted tool. The security of the underlying infrastructure
(host OS, Postgres, Redis, network) is the responsibility of whoever operates
the deployment.

## Reporting a Vulnerability

Please do **not** file a public GitHub issue for security vulnerabilities.

Report to the maintainers by email: **paul.sagar@yahoo.com**

Include:
- A description of the vulnerability and affected component
- Steps to reproduce or a proof-of-concept
- Your assessment of impact and exploitability

You will receive an acknowledgement within 2 business days and a resolution
timeline within 7 days. If the issue is confirmed, a fix will be released
before any public disclosure.

## Threat Model

A checked-in threat model is maintained at [THREAT_MODEL.md](THREAT_MODEL.md).
It documents the attack surface, prioritized threats, and current mitigations.

## Known Limitations (Current PoC State)

- **No per-record access control.** The hiveshare membership boundary is the
  only ACL. Everyone in a hiveshare can read everything in it. Do not store
  embargoed or access-controlled content in a shared hiveshare.
- **No prompt injection defense.** Hive content is returned verbatim to
  agents. See THREAT_MODEL.md for details and the planned mitigation path.
- **Invite tokens are public-endpoint credentials.** Anyone with the token
  can accept it. Share tokens only over secure channels.

## Supported Versions

This project is in active development. Only the latest commit on `main` is
supported.
