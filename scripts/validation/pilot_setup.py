#!/usr/bin/env python3
"""Prepare a NEW persistent personal-use folder; never perform fault injection."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import time
import uuid

from harness import Node,pair,prepare_output


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output",required=True,type=Path)
    args=parser.parse_args()
    prepare_output(args.output)
    report={"purpose":"prepared owner pilot; automated preparation is not owner adoption",
            "started_utc":time.strftime("%Y-%m-%dT%H:%M:%SZ",time.gmtime()),"hosts":[],"success":False,
            "network":"Dedicated SSH forwarding via workstation's existing VPS route. Workstation gateway must stay on; no firewall/VPN settings changed",
            "owner_use":"pending","fault_injection":"forbidden; pilot uses .orbit-pilot rather than disposable marker"}
    nodes=[Node("laptop","laptop","pilot"),Node("rpi","pi","pilot"),Node("vps","vps","pilot")]
    gateway=Node("local","gateway","pilot")
    folder=uuid.uuid4().hex*2
    try:
        for node in nodes:
            node.setup(folder)
            entry={"host":node.host,"role":node.role,"root":node.root,"data":node.root+"/data",
                   "device":node.device,"binary_sha256":node.binary_hash,"inventory":node.inventory}
            report["hosts"].append(entry)
        pair(nodes)
        a,b,c=nodes
        gateway.root=gateway.call("create",pilot_name="OrbitPilotGateway-20261001")["root"]
        addresses={a.role:"192.168.88.83",b.role:"192.168.88.63",c.role:"127.0.0.1"}
        ports={n.role:n.call("reserve-port",address=addresses[n.role])["port"] for n in nodes}
        forward=gateway.call("reserve-port")["port"]
        client_forward={a.role:a.call("reserve-port")["port"],b.role:b.call("reserve-port")["port"]}
        reverse=[c.call("reserve-port")["port"] for _ in range(2)]
        if reverse[0]==reverse[1]: raise RuntimeError("allocated forwarding ports collide")
        unit="orbit-pilot-gateway-"+gateway.token+".service"
        command=["/usr/bin/ssh","-N","-o","BatchMode=yes","-o","ExitOnForwardFailure=yes",
                 "-o","ServerAliveInterval=15","-o","ServerAliveCountMax=3",
                 "-L",f"127.0.0.1:{forward}:127.0.0.1:{ports[c.role]}",
                 "-R",f"127.0.0.1:{reverse[0]}:{addresses[a.role]}:{ports[a.role]}",
                 "-R",f"127.0.0.1:{reverse[1]}:{addresses[b.role]}:{ports[b.role]}","vps"]
        text="[Unit]\nDescription=Orbit personal pilot private network bridge\nAfter=network.target\n[Service]\nExecStart="+" ".join(command)+"\nRestart=always\nRestartSec=5\nWorkingDirectory="+gateway.root+"\n[Install]\nWantedBy=default.target\n"
        gateway.put(unit,text.encode())
        target=Path(gateway.root)/unit
        for argv in [["link",str(target)],["daemon-reload"],["enable","--now",unit]]:
            subprocess.run(["systemctl","--user",*argv],check=True,capture_output=True,text=True)
        report["gateway"]={"root":gateway.root,"unit":unit,"state":subprocess.check_output(["systemctl","--user","is-active",unit],text=True).strip()}
        report["gateway"]["client_units"]=[]
        for node in [a,b]:
            client_unit="orbit-pilot-gateway-"+node.role+"-"+gateway.token+".service"
            cmd=["/usr/bin/ssh","-N","-o","BatchMode=yes","-o","ExitOnForwardFailure=yes",
                 "-o","ServerAliveInterval=15","-o","ServerAliveCountMax=3","-R",
                 f"127.0.0.1:{client_forward[node.role]}:127.0.0.1:{forward}",node.host]
            client_text=text.replace(" ".join(command)," ".join(cmd))
            gateway.put(client_unit,client_text.encode())
            for argv in [["link",str(Path(gateway.root)/client_unit)],["daemon-reload"],["enable","--now",client_unit]]:
                subprocess.run(["systemctl","--user",*argv],check=True,capture_output=True,text=True)
            report["gateway"]["client_units"].append(client_unit)
        urls={a.role:{b.role:f"https://{addresses[b.role]}:{ports[b.role]}",c.role:f"https://127.0.0.1:{client_forward[a.role]}"},
              b.role:{a.role:f"https://{addresses[a.role]}:{ports[a.role]}",c.role:f"https://127.0.0.1:{client_forward[b.role]}"},
              c.role:{a.role:f"https://127.0.0.1:{reverse[0]}",b.role:f"https://127.0.0.1:{reverse[1]}"}}
        for node,entry in zip(nodes,report["hosts"]):
            peers=[{"folder":folder,"device":p.device,"url":urls[node.role][p.role],"certificate":"../"+p.role+".pem"}
                   for p in nodes if p is not node]
            node.put("state/peers.json",json.dumps({"format_version":1,"peers":peers}).encode())
            node.put("state/limits.json",json.dumps({"format_version":1,"data_budget_bytes":1024**3,
                   "metadata_budget_bytes":256*1024**2,"free_space_reserve_bytes":512*1024**2}).encode())
            node.cli("config","validate")
            entry["installation"]=node.call("service-install",template=Path("packaging/systemd/orbit.service").read_text(),
                                            peer_listen=addresses[node.role]+":"+str(ports[node.role]),profile="pi" if node.role=="pi" else "laptop")
        sentinel=b"Automated setup check; this is not a personal-use entry.\n"
        a.put("data/setup-check.txt",sentinel)
        expected=hashlib.sha256(sentinel).hexdigest()
        deadline=time.monotonic()+90
        while time.monotonic()<deadline:
            try:
                if all(n.call("hash",path="setup-check.txt")["sha256"]==expected for n in nodes): break
            except RuntimeError: pass
            time.sleep(1)
        else: raise RuntimeError("ordinary background edit did not reach all native pilot hosts")
        report["automated_setup_check"]={"sha256":expected,"all_three_native_hosts":True,"manual_scan_or_sync":False}
        reverse_content=b"Automated VPS-origin setup check; owner use is still pending.\n"
        c.put("data/relay-setup-check.txt",reverse_content)
        expected_reverse=hashlib.sha256(reverse_content).hexdigest()
        deadline=time.monotonic()+90
        while time.monotonic()<deadline:
            try:
                if all(n.call("hash",path="relay-setup-check.txt")["sha256"]==expected_reverse for n in nodes):break
            except RuntimeError:pass
            time.sleep(1)
        else:raise RuntimeError("VPS-origin background edit did not reach all native pilot hosts")
        report["automated_reverse_setup_check"]={"sha256":expected_reverse,"all_three_native_hosts":True,"manual_scan_or_sync":False}
        for node,entry in zip(nodes,report["hosts"]):
            entry["service"]=node.call("service-check")
        report["folder"]=folder
        report["success"]=True
        report["ready_utc"]=time.strftime("%Y-%m-%dT%H:%M:%SZ",time.gmtime())
        print("PASS: dedicated personal-use folders and background services ready; actual owner use remains pending",flush=True)
    finally:
        (args.output/"pilot-setup.json").write_text(json.dumps(report,indent=2)+"\n")


if __name__=="__main__": main()
