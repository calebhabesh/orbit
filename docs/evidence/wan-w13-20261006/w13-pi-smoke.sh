#!/bin/bash
# W13 arm64 smoke: packaged orbit-net in a marked disposable directory on a
# high port. No existing service, firewall, folder or sysctl is touched.
set -euo pipefail
ARCHIVE="$1"; IP="$2"
ROOT=$(mktemp -d /tmp/orbit-w13-smoke-XXXXXX); chmod 700 "$ROOT"
echo "filesync test data only" > "$ROOT/.filesync-disposable"
trap 'kill $PID 2>/dev/null || true; wait $PID 2>/dev/null || true; case "$ROOT" in /tmp/orbit-w13-smoke-*) [ -f "$ROOT/.filesync-disposable" ] && rm -rf "$ROOT";; esac' EXIT
cd "$ROOT"; tar -xzf "$ARCHIVE"; BIN=$(echo "$ROOT"/orbit-net-*/bin/orbit-net)
mkdir -m 700 op op/tls
"$BIN" version
PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("'"$IP"'",0));print(s.getsockname()[1])')
UDP=$(python3 -c 'import socket;s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(("'"$IP"'",0));print(s.getsockname()[1])')
MPORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')
AUTH=$("$BIN" keygen --out op/authority.key); SVC=$("$BIN" keygen --out op/service.key)
cat > op/template.json <<J
{"version":"1","operator":"W13 Pi smoke","service_key":"$SVC","epoch":"1","origins":["https://$IP:$PORT","wss://$IP:$PORT"],"stun":["$IP:$UDP"],"privacy":"Smoke test only; memory-only metadata; no folder data."}
J
"$BIN" profile sign --authority-key op/authority.key --template op/template.json --environment self_hosted --valid-for 1h --out op/profile.json | sed -n '1,3p'
"$BIN" profile verify --profile op/profile.json --authority "$AUTH" >/dev/null
cert() { # $1 name, $2 IP SAN
  openssl genpkey -algorithm ed25519 -out op/tls/$1.key 2>/dev/null
  openssl req -new -key op/tls/$1.key -subj "/CN=$2" -out op/$1.csr 2>/dev/null
  printf "subjectAltName=IP:%s\nextendedKeyUsage=serverAuth\n" "$2" > op/$1.ext
  openssl x509 -req -in op/$1.csr -CA op/ca.pem -CAkey op/ca.key -CAcreateserial -days 2 -extfile op/$1.ext -out op/$1.pem 2>/dev/null
  cat op/$1.pem op/ca.pem > op/tls/fullchain.pem; cp op/tls/$1.key op/tls/privkey.pem; chmod 600 op/tls/*
}
openssl genpkey -algorithm ed25519 -out op/ca.key 2>/dev/null
openssl req -x509 -new -key op/ca.key -subj "/CN=W13 Pi CA" -days 2 -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign,digitalSignature -out op/ca.pem 2>/dev/null
cert leaf1 "$IP"
cat > op/serve.json <<J
{"listen":"$IP:$PORT","origin":"https://$IP:$PORT","profile":"$ROOT/op/profile.json","service_key":"$ROOT/op/service.key","tls_cert":"$ROOT/op/tls/fullchain.pem","tls_key":"$ROOT/op/tls/privkey.pem","stun_listen":"$IP:$UDP","metrics_listen":"127.0.0.1:$MPORT","relay_bps":8388608,"relay_device_bps":4194304,"relay_session_bytes":1073741824}
J
chmod 600 op/*.json op/*.key
"$BIN" serve --config op/serve.json --check
"$BIN" serve --config op/serve.json > serve.log 2>&1 & PID=$!
for i in $(seq 50); do grep -q "orbit-net: serving" serve.log && break; sleep 0.1; done
cat serve.log
curl -fsS "http://127.0.0.1:$MPORT/healthz"
BODY='{"version":"1","profile":"'$(printf '0%.0s' $(seq 64))'","sender":"'$(printf '1%.0s' $(seq 64))'","sender_pin":"'$(printf '2%.0s' $(seq 64))'","certificate_der":""}'
echo "challenge with unknown profile: $(curl -sS --cacert op/ca.pem -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d "$BODY" "https://$IP:$PORT/network/v1/challenge")"
S1=$(openssl s_client -connect "$IP:$PORT" -CAfile op/ca.pem -verify_return_error </dev/null 2>/dev/null | openssl x509 -noout -serial)
cert leaf2 "$IP"; kill -HUP $PID; sleep 1
S2=$(openssl s_client -connect "$IP:$PORT" -CAfile op/ca.pem -verify_return_error </dev/null 2>/dev/null | openssl x509 -noout -serial)
echo "serial before=$S1 after-SIGHUP=$S2"; [ "$S1" != "$S2" ]
python3 - "$IP" "$UDP" <<'P'
import socket, struct, os, sys, time
s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM); s.bind((sys.argv[1],0)); s.settimeout(0.5)
for _ in range(100): s.sendto(struct.pack("!HHI",1,0,0x2112A442)+os.urandom(12),(sys.argv[1],int(sys.argv[2])))
n=0; big=0
try:
    while True:
        d=s.recv(2048); n+=1; big=max(big,len(d))
except socket.timeout: pass
print(f"STUN answers={n} largest={big}B for 100 requests"); assert 0<n<=20 and big<=60
P
echo "fds=$(ls /proc/$PID/fd | wc -l) $(grep VmRSS /proc/$PID/status)"
curl -fsS "http://127.0.0.1:$MPORT/metrics" | grep -E '^orbit_net_(profile_valid|certificate_reloads_total|refusals_total\{reason="untrusted"\}|stun_answered_total|stun_dropped_total\{reason="rate"\}) '
kill -TERM $PID; wait $PID; echo "exit=$?"; tail -1 serve.log
