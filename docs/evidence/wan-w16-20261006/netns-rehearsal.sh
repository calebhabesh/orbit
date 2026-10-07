set -u
S=$1; R=<repo>/scripts/validation/wan_netns.sh
mount -t overlay overlay -o lowerdir=/etc,upperdir=$S/up,workdir=$S/work /etc
mount -t tmpfs tmpfs /run
mkdir -p /run/netns
sysctl -qw net.ipv4.ip_forward=1
ip link set lo up
# Fake "internet": uplink0 <-> peer namespace at 198.51.100.1; fake tunnel: tun9 <-> peer at 100.100.0.1
ip netns add inet; ip link add uplink0 type veth peer name inet0; ip link set inet0 netns inet
ip addr add 198.51.100.2/24 dev uplink0; ip link set uplink0 up
ip -n inet addr add 198.51.100.1/24 dev inet0; ip -n inet link set inet0 up; ip -n inet link set lo up
ip netns add tsnet; ip link add tun9 type veth peer name ts0; ip link set ts0 netns tsnet
ip addr add 100.100.0.2/24 dev tun9; ip link set tun9 up
ip -n tsnet addr add 100.100.0.1/24 dev ts0; ip -n tsnet link set ts0 up
ip route add default via 198.51.100.1
ip -n inet route add default dev inet0 2>/dev/null; ip -n tsnet route add default dev ts0 2>/dev/null
iptables -P FORWARD DROP
echo "--- refusal without marker"; $R up w16t uplink0 16 tcp/8443; echo "exit=$?"
echo "--- up"; ORBIT_W16_DISPOSABLE=1 $R up w16t uplink0 16 tcp/8443 || exit 1
echo "--- duplicate"; ORBIT_W16_DISPOSABLE=1 $R up w16t uplink0 16; echo "exit=$?"
$R status w16t
cat /etc/netns/orbit-w16t/resolv.conf
echo "--- links inside"; ip -n orbit-w16t -br link
echo "--- to internet"; ip netns exec orbit-w16t ping -c1 -W1 198.51.100.1 >/dev/null && echo internet-ok || echo internet-FAIL
echo "--- to tunnel peer (must fail)"; ip netns exec orbit-w16t ping -c1 -W1 100.100.0.1 >/dev/null && echo tunnel-LEAK || echo tunnel-blocked
# Host port admission
python3 -c 'import socket,threading
for p in (8443,9464):
  s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(("10.231.16.1",p));s.listen()
  threading.Thread(target=lambda s=s:[s.accept() for _ in range(9)],daemon=True).start()
import time;time.sleep(6)' & sleep 1
for p in 8443 9464; do ip netns exec orbit-w16t timeout 2 bash -c "echo > /dev/tcp/10.231.16.1/$p" 2>/dev/null && echo "host:$p open" || echo "host:$p rejected"; done
echo "--- NAT source seen by internet"; ip netns exec inet python3 -c 'import socket;s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(("198.51.100.1",4000));s.settimeout(3);d,a=s.recvfrom(64);print("from",a[0])' & sleep 0.5; ip netns exec orbit-w16t python3 -c 'import socket;socket.socket(socket.AF_INET,socket.SOCK_DGRAM).sendto(b"x",("198.51.100.1",4000))'; wait %2 2>/dev/null; sleep 0.5
echo "--- counters"; iptables -v -S FORWARD | grep orbit
echo "--- down"; ORBIT_W16_DISPOSABLE=1 $R down w16t uplink0 16
$R status w16t; iptables -S | grep -c orbit-w16 ; iptables -t nat -S | grep -c orbit-w16; ls /etc/netns 2>&1
