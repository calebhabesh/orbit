# Security policy

Orbit is a personal portfolio project maintained on a best-effort basis.

## Reporting a vulnerability

Please report suspected vulnerabilities privately through GitHub's
**Report a vulnerability** button on the repository's Security tab. If that is
unavailable, email calebhabesh@gmail.com with "Orbit security" in the subject.
Do not open a public issue for an unfixed vulnerability.

Include the affected version (`orbit version`), the component (sync engine,
peer protocol, connection service `orbit-net`, packaging) and steps to
reproduce. Expect an acknowledgement within a week.

## Scope

- The `orbit`/`filesync` client, its peer protocol and local state handling.
- The `orbit-net` connection service and the hosted instance named in the
  release profile. Do not run load, flood or denial-of-service tests against
  the hosted service; use a self-hosted instance instead.

The trust model, threat boundaries and known limitations are described in
[operations](docs/operations.md), [protocol](docs/protocol.md) and the
[networking guide](docs/runbooks/networking.md).
