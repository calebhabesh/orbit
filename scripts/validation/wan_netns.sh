#!/usr/bin/env bash
# W16 isolated replica network: one network namespace whose only route leaves
# through the host's physical uplink via NAT. Tunnel interfaces (Tailscale,
# WireGuard) are not visible inside, and host forwarding admits the namespace
# only toward UPLINK, so ICE cannot gather or use a VPN path.
#
#   sudo ORBIT_W16_DISPOSABLE=1 wan_netns.sh up     NAME UPLINK OCTET [LOCAL_ALLOW...]
#   sudo ORBIT_W16_DISPOSABLE=1 wan_netns.sh down   NAME UPLINK OCTET
#        wan_netns.sh status NAME
#
# OCTET picks 10.231.OCTET.0/29 (host .1, namespace .2; .3-.6 for address-change
# drills). LOCAL_ALLOW entries such as tcp/8443 admit the namespace to those host
# ports (a replica on the service host); every other host port is rejected.
# Every rule carries the comment "orbit-w16-NAME" and lives in runtime tables
# only: nothing is saved to /etc/iptables, and a reboot also removes it.
set -euo pipefail

usage() { sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }
[[ $# -ge 2 ]] || usage
action=$1 name=$2
[[ $name =~ ^[a-z0-9-]{1,10}$ ]] || { echo "NAME must be 1-10 of [a-z0-9-]" >&2; exit 2; }
ns="orbit-$name" host_if="ow-$name-h" ns_if="ow-$name-n" tag="orbit-w16-$name"

if [[ $action == status ]]; then
	ip netns list | grep -qx "$ns\( .*\)\?" || { echo "absent"; exit 0; }
	echo "namespace $ns"; ip -n "$ns" -br addr; ip -n "$ns" route
	iptables -S 2>/dev/null | grep -- "$tag" || true
	iptables -t nat -S 2>/dev/null | grep -- "$tag" || true
	exit 0
fi

[[ $# -ge 4 ]] || usage
uplink=$3 octet=$4; shift 4
[[ ${ORBIT_W16_DISPOSABLE:-} == 1 ]] || { echo "refusing: set ORBIT_W16_DISPOSABLE=1 to confirm a disposable W16 environment" >&2; exit 3; }
[[ $octet =~ ^[0-9]{1,3}$ && $octet -ge 1 && $octet -le 254 ]] || { echo "OCTET must be 1-254" >&2; exit 2; }
[[ $uplink =~ ^[a-zA-Z0-9._-]{1,15}$ ]] && ip link show dev "$uplink" >/dev/null || { echo "unknown uplink $uplink" >&2; exit 2; }
subnet="10.231.$octet.0/29" host_ip="10.231.$octet.1" ns_ip="10.231.$octet.2"

# Delete each rule carrying our tag, by exact spec, from both tables.
remove_rules() {
	local table spec
	for table in filter nat; do
		while read -r spec; do
			[[ -n $spec ]] || continue
			eval "iptables -t $table ${spec/-A /-D }"
		done < <(iptables -t "$table" -S | grep -- "--comment $tag" || true)
	done
}

case $action in
up)
	if ip netns list | grep -q "^$ns\b"; then echo "refusing: $ns already exists (run down first)" >&2; exit 1; fi
	if ip -br addr | grep -q "10\.231\.$octet\."; then echo "refusing: 10.231.$octet.0/29 already in use" >&2; exit 1; fi
	[[ $(sysctl -n net.ipv4.ip_forward) == 1 ]] || { echo "refusing: net.ipv4.ip_forward is 0; this script does not change it" >&2; exit 1; }
	ip netns add "$ns"
	ip link add "$host_if" type veth peer name "$ns_if"
	ip link set "$ns_if" netns "$ns"
	ip addr add "$host_ip/29" dev "$host_if"
	ip link set "$host_if" up
	ip -n "$ns" addr add "$ns_ip/29" dev "$ns_if"
	ip -n "$ns" link set lo up
	ip -n "$ns" link set "$ns_if" up
	ip -n "$ns" route add default via "$host_ip"
	mkdir -p "/etc/netns/$ns"
	printf 'nameserver 1.1.1.1\nnameserver 9.9.9.9\n' > "/etc/netns/$ns/resolv.conf"
	# Inserted at the top so Docker/Tailscale chains never see namespace traffic.
	iptables -I FORWARD 1 -i "$host_if" ! -o "$uplink" -m comment --comment "$tag" -j REJECT
	iptables -I FORWARD 1 -i "$uplink" -o "$host_if" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "$tag" -j ACCEPT
	iptables -I FORWARD 1 -i "$host_if" -o "$uplink" -m comment --comment "$tag" -j ACCEPT
	iptables -t nat -I POSTROUTING 1 -s "$subnet" -o "$uplink" -m comment --comment "$tag" -j MASQUERADE
	iptables -I INPUT 1 -i "$host_if" -m comment --comment "$tag" -j REJECT
	iptables -I INPUT 1 -i "$host_if" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "$tag" -j ACCEPT
	for allow in "$@"; do
		[[ $allow =~ ^(tcp|udp)/[0-9]{1,5}$ ]] || { echo "bad LOCAL_ALLOW $allow" >&2; remove_rules; exit 2; }
		iptables -I INPUT 1 -i "$host_if" -p "${allow%/*}" --dport "${allow#*/}" -m comment --comment "$tag" -j ACCEPT
	done
	echo "up: $ns $ns_ip/29 -> $host_ip via $uplink"
	;;
down)
	remove_rules
	ip link del "$host_if" 2>/dev/null || true
	ip netns del "$ns" 2>/dev/null || true
	rm -f "/etc/netns/$ns/resolv.conf"; rmdir "/etc/netns/$ns" 2>/dev/null || true
	echo "down: $ns removed"
	;;
*) usage ;;
esac
