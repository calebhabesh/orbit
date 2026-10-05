# Operator Runbook: Private Network Configuration and Reachability

The owner selected [automatic native WAN networking](../orbit-wan-implementation-plan.md)
on 2026-10-05. It is planned and unimplemented; this runbook remains the current
manual/private-network path and will stay available as an Advanced alternative.
Its existing deployment instructions do not establish future direct/relay WAN evidence.

This runbook guides operators through configuring private network connectivity, verifying peer reachability, and configuring firewall rules for Orbit replication (Requirement U06, Operations Contract).

## Recommended personal deployment

For laptop/Pi/VPS use, install Tailscale directly on each participating host and
authenticate all three to the same personal tailnet. The first account sign-up
creates the tailnet; there is no Orbit account or central Orbit coordinator.
Follow the official [quickstart](https://tailscale.com/docs/how-to/quickstart)
and [Linux installation instructions](https://tailscale.com/docs/install/linux).
After installation, `sudo tailscale up` provides a browser sign-in URL.
Use `tailscale ip -4` and `tailscale status` to inspect the assigned addresses
and authenticated devices. Keep credentials and authentication links private.

Use a simple device mesh initially. Orbit needs neither an exit node nor an
advertised home subnet. A separate WireGuard gateway can continue providing
home-network access; verify coexistence using Tailscale's
[other-VPN guidance](https://tailscale.com/docs/reference/faq/other-vpns).
On hosts with existing DNS policy, explicitly review whether Tailscale should
manage DNS; numeric Orbit addresses do not require MagicDNS. Installing and
authenticating the VPN are deliberate operator steps, outside Orbit setup.

In Orbit Setup Advanced, select each host's numeric Tailscale address for peer
and enrollment listeners and advertisements, using distinct TCP ports such as
8443 and 8444. Permit those ports between the participating devices under your
tailnet and host firewall policies. Keep owner control bound to loopback.
Create/adopt the laptop's folder, invite and approve the Pi, then invite and
approve the VPS through a participating device. Joining the tailnet establishes
network reachability; Orbit's separately reviewed folder membership grants data
access. Device names do not substitute for key verification or folder consent.

The laptop is an ordinary writable replica with login startup. The Pi and VPS
are ordinary writable replicas with deliberately configured unattended startup;
they can retain and forward captured histories while the laptop is offline.
Neither decides conflict winners. A VPS is optional for ordinary two-device use.
Tailscale does not implement Orbit's capture, causal history, chunk verification,
membership, receipts, conflict resolution or durable recovery. See
[startup modes](install.md#startup-modes) for Orbit's separate service policy.

### Understanding a tailnet alongside a home WireGuard gateway

A tailnet is a persistent private device network with stable virtual addresses;
individual devices can go offline without removing their enrollment. Tailscale
uses WireGuard for encrypted transport and adds peer coordination, NAT traversal
and access policy. The operator's existing Pi WireGuard setup, described as a
WAN entry to the home LAN, serves a different routing role: a remote client sends
home-subnet traffic through the Pi gateway. Plain WireGuard can also support
other topologies; gateway versus mesh describes these deployments rather than a
limitation of the protocol. See [WireGuard in Tailscale](https://tailscale.com/docs/concepts/wireguard).

Both provide routed IP connectivity. A remote client does not join the home's
Ethernet/Wi-Fi broadcast domain merely by using either VPN; LAN broadcast and
multicast discovery do not automatically extend across the mesh. Orbit uses
explicit peer IP:ports and needs no LAN discovery. See
[Tailscale's network-layer explanation](https://tailscale.com/docs/concepts/tailscale-osi).

Connecting the Pi to the tailnet exposes the Pi under its own private address;
it does not automatically expose every home LAN device. A Tailscale
[subnet router](https://tailscale.com/docs/features/subnet-routers) can deliberately
provide that gateway role for devices without a client. Retain the existing Pi
WireGuard service initially for home-subnet access and an independent access
path. Reassess retirement only after the replacement covers those uses. Orbit's
device mesh does not require changing the existing gateway, enabling subnet
routing or selecting an exit node.

---

## 1. Network Topology and Core Architecture

Orbit synchronizes files between trusted devices over operator-provided private
network paths. Orbit supplies no discovery, NAT traversal or application relay
infrastructure and has no centralized metadata server. The underlying VPN can
carry encrypted packets directly or through a transport relay; see Tailscale's
[connection types](https://tailscale.com/docs/reference/connection-types).

- **Direct Mutual TLS Replication**: Nodes exchange causal version DAGs and content-addressed chunks over mutual TLS 1.3 using dedicated Ed25519 cryptographic keypins (Invariant I01, I07).
- **Private Network Reachability as a Prerequisite (U06)**: Participating devices must be reachable via routable IP addresses across:
  - **Local Area Networks (LAN)**: Home/office Wi-Fi or Ethernet subnets (e.g., `192.168.1.x`, `10.0.0.x`).
  - **Encrypted Overlay VPNs**: Private mesh networks such as Tailscale, WireGuard, ZeroTier, or Nebula (e.g., `100.x.y.z`).
- **No Automatic Firewall Manipulation**: In accordance with the Operations Specification (`docs/operations.md`), Orbit **never** makes automatic privileged changes to host firewalls (`ufw`, `firewalld`, `iptables`, `nftables`). All network access remains under explicit operator governance.

---

## 2. Port and Listener Specifications

Orbit separates owner control, peer replication and enrollment interfaces with strict security isolation:

| Listener | Default Binding | Protocol | Security Boundary |
| --- | --- | --- | --- |
| **Control / Web UI** | `127.0.0.1:8080` | HTTP / Loopback | Strictly loopback only. Protected by DNS-rebinding checks, one-use bootstrap tokens, and session cookies. **Never expose to external interfaces.** |
| **Peer Replication** | Explicit numeric listener (example private IP:8443) | HTTPS / mTLS | Mutual TLS with pinned Ed25519 certificates. Enforces strict body limits (16 KiB enrollment requests, bounded chunk streaming). |

| **Enrollment** | Explicit numeric listener (example private IP:8444) | HTTPS / possession proof | Invitation binds certificate, folder and reachable endpoint; authenticate inviter before capability disclosure. |

Peer/enrollment listeners are disabled until configured. Setup Advanced or reviewed
runtime settings persist listeners and numeric nonloopback advertised IP:ports.
Use existing LAN routes or Tailscale addresses/ACLs; permit both chosen TCP ports.
The owner control port remains loopback and must not be used as an invitation endpoint.

---

## 3. Host Firewall Configuration

To permit inbound peer synchronization traffic on TCP port `8443`, configure the host firewall according to your Linux distribution:

### Option A: UFW (Ubuntu / Debian / Raspberry Pi OS)

Allow inbound mTLS replication on TCP port 8443 from local LAN or VPN interfaces:
```bash
# Allow from any interface on LAN:
sudo ufw allow 8443/tcp comment "Orbit peer replication"

# Or restrict exclusively to a VPN interface (e.g. tailscale0 / wg0):
sudo ufw allow in on tailscale0 to any port 8443 proto tcp comment "Orbit on Tailscale"
sudo ufw allow in on wg0 to any port 8443 proto tcp comment "Orbit on WireGuard"
```

### Option B: Firewalld (Fedora / RHEL / Rocky Linux)

```bash
# Add port to active firewall zone:
sudo firewall-cmd --permanent --add-port=8443/tcp
sudo firewall-cmd --reload
```

### Option C: Plain `iptables` / `nftables`

```bash
# iptables rule:
sudo iptables -A INPUT -p tcp --dport 8443 -m conntrack --ctstate NEW,ESTABLISHED -j ACCEPT

# nftables rule:
sudo nft add rule inet filter input tcp dport 8443 accept
```

---

## 4. Reachability Probing and Diagnostic Verification

When pairing devices or diagnosing disconnected peers, verify network reachability using standard utilities and Orbit's built-in diagnostics:

### Step 1: Test TCP Connectivity from Remote Peer
From the remote device, verify that the host listener is reachable over the private network:
```bash
# Using netcat:
nc -zv 192.168.1.50 8443

# Or using curl to verify TLS handshake rejection (expected 400/403 without client cert):
curl -k -v https://192.168.1.50:8443/api/v1/health
```

### Step 2: Check Active Device Status in Orbit
Inspect observed peer statuses, last contact timestamps, and transport endpoints:
```bash
orbit devices list
orbit status --json
```

Output highlights:
- `endpoint`: Current advertised URL for remote peer (e.g. `https://192.168.1.50:8443` or `https://100.101.0.15:8443`).
- `last_contact`: Wall-clock time of most recent successful mutual TLS sync.
- `state`: `active`, `pending_approval`, or `retired`.

### Step 3: Update Advertised Peer Endpoints
If a device relocates to a new IP address or VPN subnet, update its endpoint mapping:
```bash
orbit devices endpoint --device <device-id> --endpoint https://100.101.0.15:8443
```
Peers immediately adopt the updated route for outbound scheduling without restarting the service or modifying causal DAGs.

---

## 5. Security Invariants and Boundaries

1. **Loopback Protection (Invariant I21)**: The operator web console and local control API bind strictly to loopback (`127.0.0.1`). Exposing the control port to public interfaces is rejected to protect local filesystem and identity keys.
2. **Capability-Gated Invitations (Invariant I23)**: Pairing invitations provide only the capability to submit bounded enrollment join requests (`<= 16 KiB`). Raw invitation tokens grant zero read/write access to folder inventory, CAS chunks, or local files.
3. **Strict Mutual Authentication**: Even on open LANs or public Wi-Fi, untrusted devices cannot read or inject file versions because every connection requires mutual TLS verification against enrolled Ed25519 key pins approved in the workspace membership chain (Invariant I07, I24).
