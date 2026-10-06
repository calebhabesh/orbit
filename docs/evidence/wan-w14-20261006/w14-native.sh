#!/bin/bash
# W14 native packaged journey: release packages with the embedded hosted profile,
# fresh disposable state on the owner's laptop (amd64) and Pi 4B (arm64), no
# profile file, no addresses. Ordinary use of the hosted service only (no fault,
# flood or abuse traffic). Only marked temporary directories are created/removed.
set -euo pipefail
DIST=${1:?dist directory}
LOCAL=$(mktemp -d /tmp/orbit-w14-native-XXXXXX); chmod 700 "$LOCAL"
echo "filesync test data only" > "$LOCAL/.filesync-disposable"
REMOTE=$(ssh rpi 'R=$(mktemp -d /tmp/orbit-w14-native-XXXXXX); chmod 700 $R; echo "filesync test data only" > $R/.filesync-disposable; echo $R')
cleanup() {
  [ -x "$LOCAL/pkg/orbit" ] && "$LOCAL/pkg/filesync" stop --state "$LOCAL/state" >/dev/null 2>&1 || true
  ssh rpi "[ -x $REMOTE/pkg/filesync ] && $REMOTE/pkg/filesync stop --state $REMOTE/state >/dev/null 2>&1 || true; case $REMOTE in /tmp/orbit-w14-native-*) [ -f $REMOTE/.filesync-disposable ] && rm -rf $REMOTE;; esac" || true
  case "$LOCAL" in /tmp/orbit-w14-native-*) [ -f "$LOCAL/.filesync-disposable" ] && rm -rf "$LOCAL";; esac
}
trap cleanup EXIT
mkdir -m 700 "$LOCAL/pkg"; tar -xzf "$DIST/orbit-v1.0.0-linux-amd64.tar.gz" -C "$LOCAL/pkg"
scp -q "$DIST/orbit-v1.0.0-linux-arm64.tar.gz" rpi:"$REMOTE/pkg.tgz"
ssh rpi "mkdir -m 700 $REMOTE/pkg && tar -xzf $REMOTE/pkg.tgz -C $REMOTE/pkg && rm $REMOTE/pkg.tgz"
A="$LOCAL/pkg/orbit"; SA="$LOCAL/state"; RA="$LOCAL/notes"
B="$REMOTE/pkg/orbit"; SB="$REMOTE/state"; RB="$REMOTE/notes"
echo "== versions"; "$A" version | tail -1; ssh rpi "$B version | tail -1"

echo "== laptop: an earlier install that chose Automatic before a profile was packaged"
ORBIT_DISABLE_PACKAGED_PROFILE=1 "$A" setup --state "$SA" --root "$RA" --label "W14 laptop" --name "W14 notes" --preview --review-file "$LOCAL/setup.json" --json >/dev/null
ORBIT_DISABLE_PACKAGED_PROFILE=1 "$A" setup --state "$SA" --request-file "$LOCAL/setup.json" --timeout 0 --json >/dev/null
for i in $(seq 50); do "$A" network status --state "$SA" --json | grep -q PROFILE_MISSING_OR_EXPIRED && break; sleep 0.2; done
"$A" network status --state "$SA" | sed -n '1p;/^Profile/p;/^Next action/p'
"$LOCAL/pkg/filesync" stop --state "$SA" >/dev/null
echo "== laptop: upgrade = same state, packaged profile now present"
T0=$(date +%s.%N)
"$A" network automatic --state "$SA"
for i in $(seq 100); do "$A" network status --state "$SA" --json | grep -q '"code":"SERVICE_READY"' && break; sleep 0.2; done
echo "ready after $(python3 -c "print(round($(date +%s.%N)-$T0,1))") s"
"$A" network status --state "$SA" | sed -n '1p;/^Operator/p;/^Profile/p;/^Packaged/p'

echo "== pairing a fresh Pi with a one-line code (join is its first setup)"
echo "laptop original" > "$RA/from-laptop.txt"
CODE=$("$A" devices invite --state "$SA" --code 2>/dev/null)
echo "code: ${CODE:0:20}... (${#CODE} chars, not recorded)"
T0=$(date +%s.%N)
ssh rpi "$B join --state $SB --root $RB --label 'W14 Pi' --name 'W14 notes' --invitation-stdin --preview --review-file $REMOTE/join.json --json >$REMOTE/join-preview.out 2>&1 && $B join --state $SB --request-file $REMOTE/join.json --timeout 0 --json >$REMOTE/join.out 2>&1; echo join-exit=\$?" <<<"$CODE" || { ssh rpi "cat $REMOTE/join-preview.out $REMOTE/join.out 2>/dev/null | head -c 2000"; exit 1; }
ssh rpi "$B network status --state $SB" | sed -n '1p;/^Operator/p;/^Profile/p'
for i in $(seq 120); do "$A" devices requests --state "$SA" | grep -q "W14 Pi" && break; sleep 0.5; done
echo "request visible on laptop after $(python3 -c "print(round($(date +%s.%N)-$T0,1))") s"
"$A" devices requests --state "$SA" --device "W14 Pi" --review-file "$LOCAL/approve.json" | sed 's/request=[0-9a-f]*/request=…/'
ssh rpi "$B status --state $SB 2>/dev/null | grep -i verification || true"
"$A" devices approve --state "$SA" --review-file "$LOCAL/approve.json" >/dev/null
echo "approved after $(python3 -c "print(round($(date +%s.%N)-$T0,1))") s"
for i in $(seq 240); do ssh rpi "cat $RB/from-laptop.txt 2>/dev/null" | grep -q "laptop original" && break; sleep 0.5; done
echo "laptop->Pi first file after $(python3 -c "print(round($(date +%s.%N)-$T0,1))") s from join"
T0=$(date +%s.%N)
ssh rpi "echo 'pi reply' > $RB/from-pi.txt"
for i in $(seq 240); do grep -q "pi reply" "$RA/from-pi.txt" 2>/dev/null && break; sleep 0.5; done
echo "Pi->laptop after $(python3 -c "print(round($(date +%s.%N)-$T0,1))") s"
sha256sum "$RA"/*.txt | sed "s|$LOCAL|LOCAL|"; ssh rpi "sha256sum $RB/*.txt" | sed "s|$REMOTE|REMOTE|"
"$A" network status --state "$SA" | grep -E '^(Connection|Device)' | sed -E 's/Device [0-9a-f]{12}[0-9a-f]*/Device …/'
