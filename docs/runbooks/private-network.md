# Operator Runbook: Private Network Configuration and Reachability

This runbook guides operators through configuring private network connectivity, verifying peer reachability, and configuring firewall rules for Orbit replication (Requirement U06, Operations Contract).

---

## 1. Network Topology and Core Architecture

Orbit synchronizes files directly between trusted devices over private network paths without third-party discovery relays, cloud intermediaries, or centralized metadata servers:

- **Direct Mutual TLS Replication**: Nodes exchange causal version DAGs and content-addressed chunks over mutual TLS 1.3 using dedicated Ed25519 cryptographic keypins (Invariant I01, I07).
- **Private Network Reachability as a Prerequisite (U06)**: Participating devices must be reachable via routable IP addresses across:
  - **Local Area Networks (LAN)**: Home/office Wi-Fi or Ethernet subnets (e.g., `192.168.1.x`, `10.0.0.x`).
  - **Encrypted Overlay VPNs**: Private mesh networks such as Tailscale, WireGuard, ZeroTier, or Nebula (e.g., `100.x.y.z`).
- **No Automatic Firewall Manipulation**: In accordance with the Operations Specification (`docs/operations.md`), Orbit **never** makes automatic privileged changes to host firewalls (`ufw`, `firewalld`, `iptables`, `nftables`). All network access remains under explicit operator governance.

---

## 2. Port and Listener Specifications

Orbit utilizes two distinct network interfaces with strict security isolation:

| Listener | Default Binding | Protocol | Security Boundary |
| --- | --- | --- | --- |
| **Control / Web UI** | `127.0.0.1:8080` | HTTP / Loopback | Strictly loopback only. Protected by DNS-rebinding checks, one-use bootstrap tokens, and session cookies. **Never expose to external interfaces.** |
| **Peer Replication** | `0.0.0.0:8443` | HTTPS / mTLS | Mutual TLS with pinned Ed25519 certificates. Enforces strict body limits (16 KiB enrollment requests, bounded chunk streaming). |

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
