# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Gonka NOP, please report it through [GitHub's private vulnerability reporting](https://github.com/inc4/gonka-nop/security/advisories/new).

**Do not open a public issue for security vulnerabilities.**

## Scope

Security issues we care about:

- Key management (account keys, consensus keys, ML operational keys)
- Port exposure and firewall bypass (Docker/iptables interactions)
- Docker Compose configurations that leak internal services
- Credential handling in config files or logs
- Command injection via user input or configuration values
- Unsafe file permissions on sensitive data

## Response

- We will acknowledge your report within 48 hours
- We will provide an initial assessment within 7 days
- We will work with you on a fix before public disclosure

## Supported Versions

Only the latest release is supported with security updates.
