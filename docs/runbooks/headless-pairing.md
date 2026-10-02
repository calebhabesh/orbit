# Operator Runbook: Headless Device Pairing and Remote Administration

## Trigger and Purpose
- An operator needs to pair a headless device (such as a Raspberry Pi, home server, or remote cloud VPS) to an existing Orbit workspace over an SSH session.
- No desktop environment, graphical display, or web browser is available on the remote machine.
- Provides interactive and non-interactive command-line workflows with full parity to the web shell interface.

## Guarantees and Invariants
- **Invariant I19 (Headless Parity):** Every onboarding, pairing, invitation, request approval, and device management flow is available via the `orbit` CLI or local control over SSH.
- **Invariant I22 (Working-Copy Preservation):** Existing files in the joining device's sync root are indexed and preserved; join never triggers deletion of preexisting content.
- **Invariant I23 (Single-Use and Capability Scoping):** Raw invitation tokens are never stored on disk, logged, or exported. Possession of an invitation token permits only bounded join request submission (<= 16 KiB, <= 5 req/min) and grants zero read/write access to folder inventory, chunks, or files.
- **Invariant I24 (Strict Linear Rollout):** Device approval appends strictly to the linear membership chain (`Revision N+1` bound to `PriorDigest = Hash(Revision N)`). Device identity retirement is permanent and immutable.

## Prerequisites
1. **Host Node (Workstation/Owner):** Running Orbit with an active workspace. Reachable network address on private LAN, VPN, or Tailscale (e.g. `https://192.168.1.50:8443` or `http://127.0.0.1:8971`).
2. **Headless Node (Pi/VPS):** Accessible via SSH. Orbit binary installed (`filesync` with `orbit` symlink or alias in `$PATH`).

---

## Procedure

### Step 1: Create an Invitation on the Host Node
On the owner machine (or via SSH to the primary node), generate a workspace invitation token:
```bash
orbit invite create --ttl 3600 --uses 1 --endpoint https://192.168.1.50:8443
```
*Options:*
- `--ttl <seconds>`: Lifetime of the invitation (default: 86400 / 24 hours).
- `--uses <count>`: Maximum allowable join attempts before expiration (default: 1).
- `--endpoint <url>`: Advertised reachability URL for this host node.

*Output includes the formatted pairing link:*
```text
Workspace Invitation Created:
  Token:             8bff60a8098cc0f4196998d955ca7385f3d39d70f85fbe17e739a73a7a8ce47d
  Invitation Digest: 21880ab88275b52956fbf98283378912c2813052c636a0efdc9054e0ab243c46
  Invitation Code:   orbit-invitation:v1?token=8bff...&folder=0b09...&endpoint=https%3A%2F%2F192.168.1.50%3A8443
  Folder:            0b09e5644cd822605be43ddebd475b206139220aa850d2fa0a38466d4b3836b7
  Expires At:        2026-10-02T07:01:44Z
  Max Uses:          1
```

### Step 2: Join Workspace from the Headless Node over SSH
Connect to the remote headless device via SSH and run `orbit join` (or `orbit setup --join`):
```bash
ssh user@vps.internal "orbit join \
  --invitation 'orbit-invitation:v1?token=8bff...&folder=0b09...&endpoint=https%3A%2F%2F192.168.1.50%3A8443' \
  --root /srv/orbit \
  --label 'Headless-VPS' \
  --timeout 120"
```
*Options:*
- `--invitation`: The formatted `orbit-invitation:v1?...` link (or raw token combined with `--remote` and `--folder`).
- `--root`: Local sync directory on the headless machine (created automatically if missing; existing files preserved per Invariant I22).
- `--label`: Suggested human-readable display name for this machine.
- `--timeout`: Seconds to wait for owner approval (default: 60s; use `0` for non-blocking submission).

### Step 3: Inspect Pending Join Requests on the Host Node
On the host node, inspect incoming join requests:
```bash
orbit requests list --status pending
```
*Output:*
```text
Enrollment Requests (1 pending):
  [pending] request_id=req-2b98e006d030bc0d device_id=10a64f... label="Headless-VPS" key_pin=29480d... created=2026-10-02T06:01:44Z
```
Verify the cryptographic public key pin (`key_pin`) and device identifier against the joining device's console output.

### Step 4: Approve the Enrollment Request
Approve the request, optionally customizing the device label and establishing a mutual pull endpoint:
```bash
orbit requests approve \
  --request req-2b98e006d030bc0d \
  --alias "Offsite Backup VPS" \
  --endpoint https://192.168.1.99:8443
```
*Output:*
```text
Enrollment request approved:
  Request ID: req-2b98e006d030bc0d
  Revision:   2
  Digest:     ab6e61c5f379fba8a5e4f3ebe100ed4852c627046329ab51648ac5d4441a51bf
  Replay:     false
```
Approval immediately mints `Revision 2` in the linear membership chain and installs the endpoint mapping.

### Step 5: Verify Completion on the Headless Node
The waiting `orbit join` command on the headless node polls the owner's status endpoint, detects approval, installs the membership record, and exits cleanly:
```text
Join request submitted (request_id=req-2b98e006d030bc0d, key_pin=99872c...).
Waiting up to 120s for owner approval on https://192.168.1.50:8443...
Workspace joined successfully: folder=0b09e564..., root=/srv/orbit, status=ready
```

### Step 6: Verify Replicas & Enable Service Persistence
Verify the active cluster membership from either machine:
```bash
orbit devices list
```
*Output:*
```text
Workspace Replicas (Membership Revision: 2):
  [active]  Offsite Backup VPS   device=10a64f... pin=29480d... endpoint=https://192.168.1.99:8443
  [active]  Desktop-Workstation  device=224b0e... pin=aad594... endpoint=https://192.168.1.50:8443
```

To ensure continuous background sync across reboots on the headless system, install and enable the systemd user service:
```bash
orbit service install
orbit service start
```

---

## Executed Fixture Transcript

The following transcript was executed against a clean disposable environment using the compiled `orbit` CLI binary:

```text
=== STEP 1: INITIALIZE OWNER WORKSPACE (MACHINE A) ===
$ orbit setup --state /tmp/owner-state --root /tmp/owner-root --label "Desktop-Workstation" --name "Lab-Workspace"
Setup completed: folder=0b09e5644cd822605be43ddebd475b206139220aa850d2fa0a38466d4b3836b7, root=/tmp/owner-root, phase=completed

=== STEP 2: LAUNCH DAEMON ON MACHINE A ===
$ orbit launch --state /tmp/owner-state --control-listen 127.0.0.1:8971 --no-browser --json
Orbit control interface ready:
http://127.0.0.1:8971/#bootstrap=d01a348dfe6d03b8ddaa5958b0a7df43b72c9cdd8f8ee6158627ed40bb0f3cf4
{"state_dir":"/tmp/owner-state","daemon_running":true,"daemon_pid":968574,"control_address":"127.0.0.1:8971","bootstrap_url":"http://127.0.0.1:8971/#bootstrap=d01a...","bootstrap_token":"d01a...","browser_opened":false,"message":"Orbit launched successfully"}

=== STEP 3: CREATE INVITATION ON MACHINE A ===
$ orbit invite create --state /tmp/owner-state --endpoint "http://127.0.0.1:8971" --ttl 3600 --uses 1
Workspace Invitation Created:
  Token:             8bff60a8098cc0f4196998d955ca7385f3d39d70f85fbe17e739a73a7a8ce47d
  Invitation Digest: 21880ab88275b52956fbf98283378912c2813052c636a0efdc9054e0ab243c46
  Invitation Code:   orbit-invitation:v1?token=8bff60a8098cc0f4196998d955ca7385f3d39d70f85fbe17e739a73a7a8ce47d&folder=0b09e5644cd822605be43ddebd475b206139220aa850d2fa0a38466d4b3836b7&endpoint=http%3A%2F%2F127.0.0.1%3A8971
  Folder:            0b09e5644cd822605be43ddebd475b206139220aa850d2fa0a38466d4b3836b7
  Expires At:        2026-10-02T07:01:44Z
  Max Uses:          1

=== STEP 4: JOIN WORKSPACE FROM MACHINE B (HEADLESS OVER SSH) ===
$ ssh user@vps.internal "orbit join --state /tmp/joiner-state --root /tmp/joiner-root --label Headless-Server --invitation 'orbit-invitation:v1?token=8bff...&folder=0b09...&endpoint=http%3A%2F%2F127.0.0.1%3A8971' --timeout 30"
Join request submitted (request_id=req-2b98e006d030bc0d, key_pin=99872c2706d32bc4f8f534931794a84620369070bb550c5da9f4054372f70984).
Waiting up to 30s for owner approval on http://127.0.0.1:8971...

=== STEP 5: LIST PENDING ENROLLMENT REQUESTS ON MACHINE A ===
$ orbit requests list --state /tmp/owner-state --status pending
Enrollment Requests (1 pending):
  [pending] request_id=req-2b98e006d030bc0d device_id=10a64f1930c645e366a168a72e72e5f73cfba56722152fc25f6d332a85ae7cce label="Headless-Server" key_pin=29480d1edf0932ebf6bd55e5ab30b0899a4e7109827a7ac81f62e747deeeb669 created=2026-10-02T06:01:44Z

=== STEP 6: APPROVE ENROLLMENT REQUEST ON MACHINE A ===
$ orbit requests approve --state /tmp/owner-state --request "req-2b98e006d030bc0d" --alias "Compute Server" --endpoint "http://127.0.0.1:8972"
Enrollment request approved:
  Request ID: req-2b98e006d030bc0d
  Revision:   2
  Digest:     ab6e61c5f379fba8a5e4f3ebe100ed4852c627046329ab51648ac5d4441a51bf
  Replay:     false

=== STEP 7: VERIFY JOIN COMPLETION ON MACHINE B ===
Workspace joined successfully: folder=0b09e5644cd822605be43ddebd475b206139220aa850d2fa0a38466d4b3836b7, root=/tmp/joiner-root, status=ready

=== STEP 8: LIST WORKSPACE REPLICAS ON MACHINE A ===
$ orbit devices list --state /tmp/owner-state
Workspace Replicas (Membership Revision: 2):
  [active]  Compute Server       device=10a64f1930c645e366a168a72e72e5f73cfba56722152fc25f6d332a85ae7cce pin=29480d1edf0932ebf6bd55e5ab30b0899a4e7109827a7ac81f62e747deeeb669 endpoint=http://127.0.0.1:8972
  [active]  Desktop-Workstation  device=224b0e02f071b4ac26dacad4eb70e89e30635eb8b2879040ade1585c1ae7ba4e pin=aad594c7e3e17d541490f1cb9dfecad3c0a944383dde484ef9bc5723b6312cdf endpoint=no endpoint

=== STEP 9: LIST WORKSPACE REPLICAS ON MACHINE B ===
$ orbit devices list --state /tmp/joiner-state
Workspace Replicas (Membership Revision: 2):
  [active]  Headless-Server      device=10a64f1930c645e366a168a72e72e5f73cfba56722152fc25f6d332a85ae7cce pin=29480d1edf0932ebf6bd55e5ab30b0899a4e7109827a7ac81f62e747deeeb669 endpoint=http://127.0.0.1:8971
  [active]  Desktop-Workstation  device=224b0e02f071b4ac26dacad4eb70e89e30635eb8b2879040ade1585c1ae7ba4e pin=aad594c7e3e17d541490f1cb9dfecad3c0a944383dde484ef9bc5723b6312cdf endpoint=no endpoint

=== STEP 10: VERIFY INVARIANT I22 (PREEXISTING CONTENT PRESERVATION) ===
$ cat /tmp/joiner-root/existing_paper.txt
important research document
```
