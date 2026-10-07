#!/bin/bash
# Same-LAN rerun of the W14 journey after the peer LAN exchange: release packages
# (epoch 2 profile) on the owner's laptop (amd64, ufw deny-incoming left
# unchanged) and Pi 4B (arm64), fresh disposable state, pairing through the
# hosted service. Ordinary use only (no fault, flood or abuse traffic). Only
# marked temporary directories are created/removed; no firewall is changed.
set -euo pipefail
DIST=${1:?dist directory}
LOCAL=$(mktemp -d /tmp/orbit-lanx-native-XXXXXX); chmod 700 "$LOCAL"
echo "filesync test data only" > "$LOCAL/.filesync-disposable"
REMOTE=$(ssh rpi 'R=$(mktemp -d /tmp/orbit-lanx-native-XXXXXX); chmod 700 $R; echo "filesync test data only" > $R/.filesync-disposable; echo $R')
cleanup() {
  [ -x "$LOCAL/pkg/filesync" ] && "$LOCAL/pkg/filesync" stop --state "$LOCAL/state" >/dev/null 2>&1 || true
  ssh rpi "[ -x $REMOTE/pkg/filesync ] && $REMOTE/pkg/filesync stop --state $REMOTE/state >/dev/null 2>&1 || true; case $REMOTE in /tmp/orbit-lanx-native-*) [ -f $REMOTE/.filesync-disposable ] && rm -rf $REMOTE;; esac" || true
  case "$LOCAL" in /tmp/orbit-lanx-native-*) [ -f "$LOCAL/.filesync-disposable" ] && rm -rf "$LOCAL";; esac
}
trap cleanup EXIT
elapsed() { python3 -c "print(round($(date +%s.%N)-$1,1))"; }
routes() { # $1 label, $2 status command output
  grep -E '^(Connection|Device)|^  Freshness' <<<"$2" | sed -E 's/[0-9a-f]{12,}/…/g; s/observed=[^;]*;?//' | sed "s/^/$1 /"
}
mkdir -m 700 "$LOCAL/pkg"; tar -xzf "$DIST/orbit-v1.0.0-linux-amd64.tar.gz" -C "$LOCAL/pkg"
scp -q "$DIST/orbit-v1.0.0-linux-arm64.tar.gz" rpi:"$REMOTE/pkg.tgz"
ssh rpi "mkdir -m 700 $REMOTE/pkg && tar -xzf $REMOTE/pkg.tgz -C $REMOTE/pkg && rm $REMOTE/pkg.tgz"
A="$LOCAL/pkg/orbit"; SA="$LOCAL/state"; RA="$LOCAL/notes"
B="$REMOTE/pkg/orbit"; SB="$REMOTE/state"; RB="$REMOTE/notes"
echo "== versions"; "$A" version | tail -1; ssh rpi "$B version | tail -1"
echo "== laptop host firewall (unchanged by this run)"
grep -E '^(ENABLED|DEFAULT_INPUT_POLICY)' /etc/ufw/ufw.conf /etc/default/ufw | sed 's|.*:||'

echo "== laptop: fresh setup (packaged profile, Automatic)"
"$A" setup --state "$SA" --root "$RA" --label "LANX laptop" --name "LANX notes" --preview --review-file "$LOCAL/setup.json" --json >/dev/null
"$A" setup --state "$SA" --request-file "$LOCAL/setup.json" --timeout 0 --json >/dev/null
for i in $(seq 150); do "$A" network status --state "$SA" --json | grep -q '"code":"SERVICE_READY"' && break; sleep 0.2; done
"$A" network status --state "$SA" | sed -n '1p;/^Operator/p;/^Profile/p'

echo "== pairing the Pi with a one-line code"
echo "laptop original" > "$RA/from-laptop.txt"
CODE=$("$A" devices invite --state "$SA" --code 2>/dev/null)
T0=$(date +%s.%N)
ssh rpi "$B join --state $SB --root $RB --label 'LANX Pi' --name 'LANX notes' --invitation-stdin --preview --review-file $REMOTE/join.json --json >$REMOTE/join-preview.out 2>&1 && $B join --state $SB --request-file $REMOTE/join.json --timeout 0 --json >$REMOTE/join.out 2>&1; echo join-exit=\$?" <<<"$CODE" || { ssh rpi "cat $REMOTE/join-preview.out $REMOTE/join.out 2>/dev/null | head -c 2000"; exit 1; }
for i in $(seq 120); do "$A" devices requests --state "$SA" | grep -q "LANX Pi" && break; sleep 0.5; done
"$A" devices requests --state "$SA" --device "LANX Pi" --review-file "$LOCAL/approve.json" >/dev/null
"$A" devices approve --state "$SA" --review-file "$LOCAL/approve.json" >/dev/null
for i in $(seq 240); do ssh rpi "cat $RB/from-laptop.txt 2>/dev/null" | grep -q "laptop original" && break; sleep 0.5; done
echo "laptop->Pi first file after $(elapsed $T0) s from join"
routes laptop "$("$A" network status --state "$SA")"
routes pi "$(ssh rpi "$B network status --state $SB")"

# The laptop's firewall drops the Pi's multicast, so LAN candidates on the laptop
# can only come from the peer LAN exchange.
echo "== waiting for LAN candidates on the laptop (exchange runs on a 15 s tick after a reached peer)"
T0=$(date +%s.%N); n=0
while [ "$(elapsed $T0 | cut -d. -f1)" -lt 360 ]; do
  n=$((n+1)); echo "edit $n" > "$RA/tick.txt"; sleep 15
  S=$("$A" network status --state "$SA")
  if grep -qE 'candidates LAN=[1-9]' <<<"$S"; then echo "laptop has the Pi's LAN candidates after $(elapsed $T0) s"; break; fi
done
routes laptop "$("$A" network status --state "$SA")"
routes pi "$(ssh rpi "$B network status --state $SB")"

echo "== transfers after the exchange"
T0=$(date +%s.%N); echo "laptop second" > "$RA/second.txt"
for i in $(seq 240); do ssh rpi "cat $RB/second.txt 2>/dev/null" | grep -q "laptop second" && break; sleep 0.5; done
echo "laptop->Pi after $(elapsed $T0) s"
T0=$(date +%s.%N); ssh rpi "echo 'pi reply' > $RB/from-pi.txt"
for i in $(seq 240); do grep -q "pi reply" "$RA/from-pi.txt" 2>/dev/null && break; sleep 0.5; done
echo "Pi->laptop after $(elapsed $T0) s"
routes laptop "$("$A" network status --state "$SA")"
routes pi "$(ssh rpi "$B network status --state $SB")"
sha256sum "$RA"/from-*.txt "$RA"/second.txt | sed "s|$LOCAL|LOCAL|"; ssh rpi "sha256sum $RB/from-*.txt $RB/second.txt" | sed "s|$REMOTE|REMOTE|"
