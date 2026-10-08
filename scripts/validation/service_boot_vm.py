#!/usr/bin/env python3
"""Native login/logout/unattended-boot drill in a disposable KVM guest.

Boots a throwaway copy-on-write guest from a checksum-verified cloud image,
installs the packaged Orbit archive as an ordinary user with the packaged
install.sh, and drives real logind sessions over SSH:

  1. login startup: `orbit service enable/start`, capture, logout stops it,
     login starts it again and captures edits made while logged out;
  2. unattended: refused without lingering, then an admin enables lingering
     (the documented owner step), the daemon survives logout, the guest
     reboots and the daemon starts and captures with no user session.

The guest is a KVM virtual machine, not physical hardware. Everything lives in
a new --work directory carrying the disposable marker; the guest and its
overlay are discarded on exit. No host service, unit or account is changed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import socket
import subprocess
import sys
import time

MARKER = ".orbit-disposable"
USER = "orbit"
ADMIN = "observer"
STATE = "/home/orbit/.local/state/orbit"
ROOT = "/home/orbit/Documents"
ORBIT = "/home/orbit/.local/bin/orbit"


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


class Guest:
    def __init__(self, work, image, port):
        self.work, self.image, self.port = work, image, port
        self.key = work / "id_ed25519"
        self.proc = None
        self.log = []

    def prepare(self):
        subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(self.key)], check=True)
        pub = (self.work / "id_ed25519.pub").read_text().strip()
        seed = self.work / "seed"
        seed.mkdir()
        (seed / "meta-data").write_text("instance-id: orbit-w17-boot\nlocal-hostname: orbit-boot\n")
        (seed / "user-data").write_text(f"""#cloud-config
users:
  - name: {ADMIN}
    shell: /bin/bash
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    ssh_authorized_keys: ["{pub}"]
  - name: {USER}
    shell: /bin/bash
    ssh_authorized_keys: ["{pub}"]
ssh_pwauth: false
package_update: false
""")
        subprocess.run(["xorriso", "-as", "mkisofs", "-quiet", "-output", str(self.work / "seed.iso"),
                        "-volid", "cidata", "-joliet", "-rock", str(seed)], check=True)
        subprocess.run(["qemu-img", "create", "-q", "-f", "qcow2", "-F", "qcow2", "-b", str(self.image),
                        str(self.work / "disk.qcow2"), "8G"], check=True)

    def boot(self, label):
        serial = open(self.work / f"serial-{label}.log", "wb")
        self.proc = subprocess.Popen([
            "qemu-system-x86_64", "-enable-kvm", "-cpu", "host", "-m", "2048", "-smp", "2",
            "-display", "none", "-serial", "stdio", "-no-reboot",
            "-drive", f"file={self.work / 'disk.qcow2'},if=virtio",
            "-drive", f"file={self.work / 'seed.iso'},media=cdrom,readonly=on",
            "-netdev", f"user,id=n0,hostfwd=tcp:127.0.0.1:{self.port}-:22",
            "-device", "virtio-net-pci,netdev=n0"], stdout=serial, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL)
        deadline = time.monotonic() + 300
        while time.monotonic() < deadline:
            if self.proc.poll() is not None:
                raise RuntimeError("guest exited during boot")
            if self.ssh(ADMIN, "systemctl is-system-running --wait || true", check=False, timeout=60).returncode == 0:
                return
            time.sleep(3)
        raise RuntimeError("guest SSH did not become ready")

    def ssh(self, user, command, check=True, timeout=120, stdin=None):
        argv = ["ssh", "-q", "-i", str(self.key), "-p", str(self.port), "-o", "BatchMode=yes",
                "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
                "-o", "ControlMaster=no", "-o", "ConnectTimeout=5", f"{user}@127.0.0.1", command]
        r = subprocess.run(argv, input=stdin, capture_output=True, text=stdin is None or isinstance(stdin, str), timeout=timeout)
        self.log.append({"at": time.time(), "user": user, "command": command, "rc": r.returncode,
                         "stdout": r.stdout[-4000:] if isinstance(r.stdout, str) else "", "stderr": r.stderr[-2000:] if isinstance(r.stderr, str) else ""})
        if check and r.returncode != 0:
            raise RuntimeError(f"{user}: {command}: rc={r.returncode} {r.stderr}")
        return r

    def copy(self, local, remote_user, remote_path):
        subprocess.run(["scp", "-q", "-i", str(self.key), "-P", str(self.port), "-o", "BatchMode=yes",
                        "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
                        str(local), f"{remote_user}@127.0.0.1:{remote_path}"], check=True)

    def wait_shutdown(self):
        try:
            self.proc.wait(timeout=180)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()
            raise RuntimeError("guest did not shut down")

    def stop(self):
        if self.proc and self.proc.poll() is None:
            self.proc.kill()
            self.proc.wait()


def admin_observe(g):
    """Observe the user's manager and daemon without creating a user session."""
    script = f"""
set -u
uid=$(id -u {USER})
user_ctl() {{ sudo -u {USER} XDG_RUNTIME_DIR=/run/user/$uid systemctl --user "$@" 2>/dev/null; }}
echo "linger=$(loginctl show-user {USER} -p Linger --value 2>/dev/null || echo none)"
echo "user_state=$(loginctl show-user {USER} -p State --value 2>/dev/null || echo absent)"
# systemd 256+ also lists a class=manager session for the user manager itself.
echo "user_sessions=$(loginctl list-sessions --no-legend | awk '$3=="{USER}" && $6=="user"' | wc -l)"
echo "unit_active=$(user_ctl is-active orbit.service || true)"
echo "unit_enabled=$(user_ctl is-enabled orbit.service || true)"
echo "unit_main_pid=$(user_ctl show -p MainPID --value orbit.service || true)"
echo "unit_restarts=$(user_ctl show -p NRestarts --value orbit.service || true)"
echo "daemon_pids=$(pgrep -u {USER} -f '{STATE} ' | tr '\\n' ' ')"
echo "agent_pid=$(cat {STATE}/.agent.pid 2>/dev/null | tr -d '\\n')"
"""
    out = g.ssh(ADMIN, "sudo bash -s", stdin=script).stdout
    return dict(line.split("=", 1) for line in out.strip().splitlines())


def history_digests(g, rel, as_admin):
    # sudo uses common-session-noninteractive (no pam_systemd): no logind session.
    cmd = f"{ORBIT} history --state {STATE} --folder Documents --json {shlex.quote(ROOT + '/' + rel)}"
    r = g.ssh(ADMIN, f"sudo -u {USER} -H {cmd}", check=False) if as_admin else g.ssh(USER, cmd, check=False)
    if r.returncode != 0:
        return []
    found = set()

    def walk(v):
        if isinstance(v, dict):
            for k, x in v.items():
                if k == "digest" and isinstance(x, str):
                    found.add(x)
                walk(x)
        elif isinstance(v, list):
            for x in v:
                walk(x)
    try:
        walk(json.loads(r.stdout))
    except ValueError:
        return []
    return sorted(found)


def wait_for(predicate, seconds, interval=1.0):
    start = time.monotonic()
    while time.monotonic() - start < seconds:
        value = predicate()
        if value:
            return value, time.monotonic() - start
        time.sleep(interval)
    return None, time.monotonic() - start


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--image", type=Path, required=True, help="verified qcow2 cloud image (read-only backing file)")
    p.add_argument("--archive", type=Path, required=True, help="packaged orbit-v*-linux-amd64.tar.gz")
    p.add_argument("--work", type=Path, required=True, help="new directory for the disposable guest")
    p.add_argument("--output", type=Path, required=True, help="new directory for sanitized results")
    p.add_argument("--port", type=int, default=0)
    a = p.parse_args()
    for path in (a.work, a.output):
        if path.exists():
            sys.exit(f"refusing existing path {path}")
    a.work.mkdir(parents=True)
    (a.work / MARKER).write_text("orbit test data only")
    a.output.mkdir(parents=True)
    port = a.port
    if not port:
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            port = s.getsockname()[1]
    g = Guest(a.work.resolve(), a.image.resolve(), port)
    results = {"packet": "W17/T13 native lifecycle", "environment": "disposable KVM guest (virtual machine, not physical hardware)",
               "image_sha256": sha256(a.image), "archive": a.archive.name, "archive_sha256": sha256(a.archive),
               "started_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"), "phases": {}, "success": False}
    phases = results["phases"]

    def edit(name, body, as_admin=False):
        data = body.encode()
        if as_admin:
            g.ssh(ADMIN, f"sudo -u {USER} tee {ROOT}/{name} >/dev/null", stdin=body)
        else:
            g.ssh(USER, f"cat > {ROOT}/{name}", stdin=body)
        return hashlib.sha256(data).hexdigest()

    def captured(name, digest, as_admin):
        return wait_for(lambda: digest in history_digests(g, name, as_admin), 90, 2)

    try:
        g.prepare()
        g.boot("first")
        results["guest"] = {k: g.ssh(ADMIN, c).stdout.strip() for k, c in {
            "os": ". /etc/os-release; echo $PRETTY_NAME", "kernel": "uname -r",
            "systemd": "systemctl --version | head -1", "virt": "systemd-detect-virt"}.items()}

        # Ordinary packaged per-user installation and first-device setup (Local only: no hosted service traffic).
        g.copy(a.archive, USER, "/home/orbit/orbit.tar.gz")
        g.ssh(USER, "mkdir -p pkg Documents && tar xzf orbit.tar.gz -C pkg && ./pkg/install.sh user")
        g.ssh(USER, "printf 'seed bytes\\n' > Documents/seed.txt")
        g.ssh(USER, f"{ORBIT} setup --state {STATE} --root {ROOT} --label VM --name Documents --connection local_only "
                    f"--preview --review-file setup.json --json >/dev/null && {ORBIT} setup --state {STATE} --request-file setup.json --timeout 60 --json")
        unit = g.ssh(USER, "cat ~/.config/systemd/user/orbit.service").stdout
        phases["install"] = {"unit_execstart": [l for l in unit.splitlines() if l.startswith("ExecStart=")]}

        # Unattended must be refused before lingering, naming the owner step.
        r = g.ssh(USER, f"{ORBIT} service enable --mode unattended --state {STATE} --json", check=False)
        phases["unattended_before_linger"] = {"rc": r.returncode, "stdout": r.stdout[-1500:], "stderr": r.stderr[-1500:],
                                             "refused": r.returncode != 0 and "enable-linger" in (r.stdout + r.stderr)}

        # Login startup through the product's own service commands. Setup left
        # a manually launched daemon running; start must not report it as the service.
        en = g.ssh(USER, f"{ORBIT} service enable --mode login --state {STATE} --json", check=False)
        refused = g.ssh(USER, f"{ORBIT} service start --state {STATE} --json", check=False)
        before = admin_observe(g)
        stop = g.ssh(USER, f"{ORBIT} stop --state {STATE}", check=False)
        st = g.ssh(USER, f"{ORBIT} service start --state {STATE} --json", check=False)
        obs = admin_observe(g)
        status = g.ssh(USER, f"{ORBIT} service status --state {STATE} --json", check=False)
        d1 = edit("login-1.txt", "edited during first login\n")
        hit, t = captured("login-1.txt", d1, False)
        phases["login_enable"] = {"enable_rc": en.returncode, "enable": en.stdout[-1500:] + en.stderr[-800:],
                                  "start_with_manual_daemon_rc": refused.returncode,
                                  "start_with_manual_daemon_refused": refused.returncode != 0 and "MANUAL_DAEMON_RUNNING" in refused.stdout + refused.stderr,
                                  "observed_before_stop": before, "stop_rc": stop.returncode,
                                  "start_rc": st.returncode, "start": st.stdout[-1500:] + st.stderr[-800:],
                                  "observed": obs, "unit_owns_state": obs["unit_active"] == "active" and obs["unit_main_pid"] == obs["agent_pid"],
                                  "status": status.stdout[-1500:], "capture": bool(hit), "capture_s": round(t, 1)}

        # Logout without lingering: the user manager and its daemon stop.
        g.ssh(USER, "true")  # no lingering SSH multiplexing; every session above has exited
        stopped, t = wait_for(lambda: (lambda o: o if o["daemon_pids"].strip() == "" and o["user_sessions"] == "0" else None)(admin_observe(g)), 60, 2)
        d_offline = edit("while-logged-out.txt", "written by an administrator while orbit was logged out\n", as_admin=True)
        after_write = admin_observe(g)
        phases["logout_without_linger"] = {"stopped": bool(stopped), "stop_s": round(t, 1), "observed": stopped or admin_observe(g),
                                           "admin_write_created_no_session": after_write["user_sessions"] == "0" and after_write["daemon_pids"].strip() == ""}

        # Next login starts the daemon again and captures the edit made while logged out.
        started = time.monotonic()
        login_obs = admin_observe(g)  # before any user session
        r = g.ssh(USER, f"sleep 1; pgrep -u {USER} -f '{STATE} ' | head -1")
        hit, t = captured("while-logged-out.txt", d_offline, False)
        phases["login_again"] = {"before": login_obs, "daemon_pid_in_session": r.stdout.strip(),
                                 "offline_edit_captured": bool(hit), "capture_s": round(time.monotonic() - started, 1)}

        # Unattended: the documented owner step, then the product's unattended enable.
        g.ssh(ADMIN, f"sudo loginctl enable-linger {USER}")
        un = g.ssh(USER, f"{ORBIT} service enable --mode unattended --state {STATE} --json", check=False)
        status = g.ssh(USER, f"{ORBIT} service status --state {STATE} --json", check=False)
        survived, t = wait_for(lambda: None, 20)  # outlast logind's user stop delay
        obs = admin_observe(g)
        phases["linger_logout"] = {"unattended_enable_rc": un.returncode, "unattended_enable": un.stdout[-1500:] + un.stderr[-800:],
                                   "status": status.stdout[-1500:], "observed_after_logout": obs,
                                   "survived_logout": obs["daemon_pids"].strip() != "" and obs["user_sessions"] == "0"}

        # Reboot; observe only through the administrator account.
        g.ssh(ADMIN, "sudo systemctl poweroff", check=False)
        g.wait_shutdown()
        boot_started = time.monotonic()
        g.boot("reboot")
        running, t = wait_for(lambda: (lambda o: o if o["daemon_pids"].strip() else None)(admin_observe(g)), 120, 2)
        d_boot = edit("after-boot.txt", "written after an unattended boot with nobody logged in\n", as_admin=True)
        hit, ct = captured("after-boot.txt", d_boot, True)
        final = admin_observe(g)
        phases["unattended_boot"] = {"daemon_running": bool(running), "since_boot_ready_s": round(t, 1),
                                     "observed": running or final, "capture": bool(hit), "capture_s": round(ct, 1),
                                     "final": final, "no_user_session": final["user_sessions"] == "0",
                                     "boot_to_capture_s": round(time.monotonic() - boot_started, 1)}
        phases["unattended_boot"]["sessions_raw"] = g.ssh(ADMIN, "loginctl list-sessions --no-legend").stdout
        phases["unattended_boot"]["unit_journal"] = g.ssh(ADMIN, "sudo journalctl -b -o short-monotonic --no-pager _SYSTEMD_USER_UNIT=orbit.service | tail -20").stdout
        integrity = g.ssh(ADMIN, f"sudo -u {USER} -H {ORBIT} doctor --state {STATE} --json", check=False)
        phases["doctor_after_boot"] = {"rc": integrity.returncode, "stdout": integrity.stdout[-3000:]}

        results["success"] = all([
            phases["unattended_before_linger"]["refused"],
            phases["login_enable"]["enable_rc"] == 0, phases["login_enable"]["start_with_manual_daemon_refused"],
            phases["login_enable"]["stop_rc"] == 0, phases["login_enable"]["start_rc"] == 0,
            phases["login_enable"]["unit_owns_state"], phases["login_enable"]["capture"],
            phases["logout_without_linger"]["stopped"], phases["logout_without_linger"]["admin_write_created_no_session"],
            phases["login_again"]["before"]["daemon_pids"].strip() == "", phases["login_again"]["daemon_pid_in_session"] != "",
            phases["login_again"]["offline_edit_captured"],
            phases["linger_logout"]["unattended_enable_rc"] == 0, phases["linger_logout"]["survived_logout"],
            phases["unattended_boot"]["daemon_running"], phases["unattended_boot"]["capture"], phases["unattended_boot"]["no_user_session"],
            phases["unattended_boot"]["final"]["unit_main_pid"] == phases["unattended_boot"]["final"]["agent_pid"],
        ])
    except Exception as e:  # retain partial observations for diagnosis
        results["error"] = repr(e)
    finally:
        g.stop()
        results["ended_at"] = time.strftime("%Y-%m-%dT%H:%M:%S%z")
        (a.output / "service-boot-vm.json").write_text(json.dumps(results, indent=2) + "\n")
        (a.output / "commands.jsonl").write_text("".join(json.dumps(x) + "\n" for x in g.log))
        for log in a.work.glob("serial-*.log"):
            shutil.copy(log, a.output / log.name)
        if (a.work / MARKER).exists():
            shutil.rmtree(a.work)
    print(("PASS" if results["success"] else "FAIL") + " native login/logout/unattended boot (KVM guest)")
    sys.exit(0 if results["success"] else 1)


if __name__ == "__main__":
    main()
