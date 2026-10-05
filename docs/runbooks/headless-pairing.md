# Headless pairing over SSH

SSH with a PTY can run `orbit` for the full Create/Join and everyday workflows;
no browser or desktop is needed. Piped commands use status/JSON without prompts.
Use [terminal onboarding](terminal-onboarding.md) for keyboard approval/sharing
and [operator guide](terminal-operator.md) for reviewed CLI setup/join.

Configure reachable LAN or existing Tailscale peer/enrollment endpoints before
inviting. Loopback owner control is not a remote invitation endpoint. On the
inviter, use Add device, deliberately reveal/save a private invitation, and
transfer that owner-only file through your existing private channel. On the
receiver use the hidden invitation field or `--invitation-file`, review existing
root contents, then submit the durable request. The inviter compares the exact
verification code before approving the exact folder/device/key request.

Invitation possession authorizes bounded enrollment submission, not data access.
Private pending preparations may retain the capability until acknowledged
submission; automatic status/diagnostic output must not reveal it. Do not put
capabilities in SSH command arguments, shell history or copied transcripts.
Delayed approvals and daemon/client restarts reuse the same request. Waiting,
membership, capture, transfer and local readiness are separate phases.

A second folder requires its own invitation/root consent and reuses the enrolled
key; separate approval does not reset identity. Use `orbit status --json` to
inspect qualified observations and `orbit doctor` for safe next actions. An
offline receipt describes what was durably stored when recorded. It does not
prove current working bytes or onward delivery.

For background service paths and explicit login/unattended prerequisites, see
[installation](install.md); for listeners/ports and routing see
[private network](private-network.md). Orbit does not change host firewall,
VPN, lingering or privileged service policy automatically.
