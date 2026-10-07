"""W16 refusal and evidence oracles; no changes to host networking."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from wan_native import WANNode, account_rules, check_routes, classify_paths, counter_delta, parse_isolation, parse_paths, path_delta, check_wg6, network_check, run, transfer, validate_topology
from wan_native_agent import wan_dispatch, wan_inventory
from host_agent import dispatch


class NativeSafetyTest(unittest.TestCase):
    def facts(self):
        return {'links': [{'ifname': 'eth0', 'link_type': 'ether', 'kind': '', 'flags': ['UP']}],
                'routes': {'8.8.8.8': [{'dev': 'eth0'}]}}

    def topology(self):
        return {'hosts': [{'host': 'laptop', 'role': 'Laptop', 'physical_network': 'home'},
                          {'host': 'vps', 'role': 'VPS', 'physical_network': 'OCI'}]}

    def test_third_host_requires_owner_shell_and_safe_alias(self):
        good = dict(self.topology(), third={'host': 'laptop', 'role': 'Laptop', 'physical_network': 'wifi',
                                             'shell': '/home/u/orbit-w16/run/shell.sock'})
        validate_topology(good)
        for bad in ({'host': 'laptop', 'role': 'Laptop', 'physical_network': 'wifi'},
                    {'host': '-oProxyCommand=x', 'role': 'L', 'physical_network': 'w', 'shell': '/s'},
                    {'host': 'laptop', 'role': 'L', 'physical_network': 'w', 'shell': 'relative.sock'},
                    {'host': 'laptop', 'role': 'L', 'physical_network': 'w', 'shell': '/s', 'netns': 'orbit-x'}):
            with self.assertRaises(RuntimeError):
                validate_topology(dict(self.topology(), third=bad))

    def test_namespace_shell_runs_worker_for_owner_only(self):
        import socket, subprocess, sys, time
        from wan_native import SHELL_CLIENT
        with tempfile.TemporaryDirectory() as tmp:
            os.chmod(tmp, 0o700)
            sock = os.path.join(tmp, 'shell.sock')
            server = subprocess.Popen([sys.executable, str(Path(__file__).with_name('wan_netns_shell.py')), sock, '--idle', '20'])
            try:
                for _ in range(100):
                    if os.path.exists(sock):
                        break
                    time.sleep(.05)
                self.assertEqual(os.stat(sock).st_mode & 0o777, 0o600)
                worker = 'import json,sys; r=json.load(sys.stdin); print(json.dumps({"echo": r["x"]})); sys.exit(r["code"])'
                for code in (0, 3):
                    result = subprocess.run([sys.executable, '-c', SHELL_CLIENT, sock], capture_output=True, text=True,
                                            input=json.dumps({'worker': worker, 'request': {'x': 'ok', 'code': code}}))
                    self.assertEqual(result.returncode, code)
                    self.assertEqual(json.loads(result.stdout), {'echo': 'ok'})
            finally:
                server.kill()
                server.wait()
        with tempfile.TemporaryDirectory() as tmp:
            os.chmod(tmp, 0o755)
            refused = subprocess.run([sys.executable, str(Path(__file__).with_name('wan_netns_shell.py')),
                                      os.path.join(tmp, 's.sock'), '--idle', '1'], capture_output=True, text=True)
            self.assertNotEqual(refused.returncode, 0)

    def test_tui_frames_redact_root_capability_and_addresses(self):
        from wan_tui_phase import redact
        class H:
            root = Path('/home/u/filesync-validation-abc')
        text = 'Root: /home/u/filesync-va\nlidation-abc/data cap=SECRETCAP 203.0.113.7:3478'
        out = redact(text, H, ['SECRETCAP'])
        self.assertNotIn('SECRETCAP', out)
        self.assertNotIn('203.0.113.7', out)
        self.assertIn('<root>/data', out)

    def test_vpn_link_kinds_and_obscure_names_refused(self):
        for kind, name in [('wireguard', 'innocent'), ('tun', 'innocent'), ('', 'tailscale0'),
                           ('gre', 'innocent'), ('vxlan', 'innocent')]:
            facts = self.facts()
            facts['links'].append({'ifname': name, 'kind': kind, 'flags': ['UP']})
            with self.assertRaises(RuntimeError):
                check_routes(facts)
        check_routes(self.facts())

    def test_missing_private_and_tunnel_routes_refused(self):
        for routes in [{}, {'100.64.1.2': [{'dev': 'eth0'}]}, {'192.168.1.2': [{'dev': 'eth0'}]},
                       {'8.8.8.8': []}, {'8.8.8.8': [{'dev': 'unknown'}]}]:
            with self.assertRaises(RuntimeError):
                check_routes({**self.facts(), 'routes': routes})

    def test_actual_open_gate_cannot_be_bypassed_with_unrelated_closed_text(self):
        with self.assertRaisesRegex(RuntimeError, 'WG6 remains open'):
            check_wg6('### WG6 W13 outcome — open, 2026-10-06\nWG6 stays open.\n')
        with self.assertRaises(RuntimeError):
            check_wg6('WG1 closed\n### WG6 W13 outcome — open\n')
        check_wg6('### WG6 W13 outcome — closed, 2026-10-07\nRestoration and alert evidence recorded.\n')

    def test_topology_refuses_shared_network_and_ssh_option_injection(self):
        validate_topology(self.topology())
        for field, value in [('host', '-L8080'), ('host', 'vps;reboot'), ('physical_network', 'home')]:
            topology = self.topology()
            topology['hosts'][1][field] = value
            with self.assertRaises(RuntimeError):
                validate_topology(topology)

    def test_numeric_probe_rejects_shell_and_loopback_before_commands(self):
        with patch('wan_native_agent.subprocess.check_output') as command:
            for value in ('8.8.8.8;reboot', '127.0.0.1', '::', 'localhost'):
                with self.assertRaises((RuntimeError, ValueError)):
                    wan_inventory([value])
            command.assert_not_called()

    def test_ssh_never_forwards_or_reuses_a_master_and_errors_hide_secrets(self):
        n = WANNode('vps', 'VPS')
        with patch('wan_native.subprocess.run') as command:
            command.return_value = argparse.Namespace(returncode=1, stdout='secret-invitation', stderr='secret')
            with self.assertRaises(RuntimeError) as error:
                n.call('inventory')
            self.assertNotIn('secret', str(error.exception))
            argv = command.call_args.args[0]
            for expected in ('ClearAllForwardings=yes', 'ControlMaster=no', 'ControlPath=none'):
                self.assertIn(expected, argv)
            self.assertNotIn('-L', argv)
            self.assertNotIn('-R', argv)

    RULES = ['-A INPUT -i ow-w16-h -m comment --comment orbit-w16-w16 -c 3 200 -j REJECT --reject-with icmp-port-unreachable',
             '-A FORWARD -i ow-w16-h -o eth0 -m comment --comment orbit-w16-w16 -c 30 3082 -j ACCEPT',
             '-A FORWARD -i eth0 -o ow-w16-h -m conntrack --ctstate RELATED,ESTABLISHED -m comment --comment orbit-w16-w16 -c 22 9140 -j ACCEPT',
             '-A FORWARD -i ow-w16-h ! -o eth0 -m comment --comment orbit-w16-w16 -c 1 84 -j REJECT --reject-with icmp-port-unreachable',
             '-A POSTROUTING -s 10.231.17.0/29 -o eth0 -m comment --comment orbit-w16-w16 -c 4 248 -j MASQUERADE']
    UPLINKS = [{'ifname': 'eth0', 'link_type': 'ether'}, {'ifname': 'tailscale0', 'link_type': 'none'},
               {'ifname': 'docker0', 'link_type': 'ether', 'linkinfo': {'info_kind': 'bridge'}}]

    def netns_facts(self):
        return {'links': [{'ifname': 'lo', 'link_type': 'loopback', 'kind': '', 'flags': ['UP']},
                          {'ifname': 'ow-w16-n', 'link_type': 'ether', 'kind': 'veth', 'flags': ['UP']}],
                'routes': {'132.145.111.200': [{'dev': 'ow-w16-n'}]}}

    def test_namespace_isolation_requires_confining_rules_and_physical_uplink(self):
        summary = parse_isolation(self.RULES, self.UPLINKS)
        self.assertEqual(summary['uplink'], 'eth0')
        self.assertEqual(summary['counters']['egress_accept'], {'packets': 30, 'bytes': 3082})
        check_routes(self.netns_facts(), summary)
        later = parse_isolation([r.replace('-c 30 3082', '-c 40 9082') for r in self.RULES], self.UPLINKS)
        self.assertEqual(counter_delta(summary, later)['egress_accept'], {'packets': 10, 'bytes': 6000})
        # Missing reject-other, NAT to a tunnel/bridge, or mixed uplinks all refuse.
        for rules in ([r for r in self.RULES if '! -o' not in r],
                      [r.replace('eth0', 'tailscale0') for r in self.RULES],
                      [r.replace('eth0', 'docker0') for r in self.RULES],
                      [r.replace('-o eth0 -m comment --comment orbit-w16-w16 -c 4', '-o wlan0 -m comment --comment orbit-w16-w16 -c 4') for r in self.RULES],
                      []):
            with self.assertRaises(RuntimeError):
                parse_isolation(rules, self.UPLINKS)

    def test_namespace_must_hide_tunnels_and_route_via_its_veth(self):
        summary = parse_isolation(self.RULES, self.UPLINKS)
        leaky = self.netns_facts()
        leaky['links'].append({'ifname': 'tailscale0', 'link_type': 'none', 'kind': '', 'flags': ['UP']})
        wrong_route = {**self.netns_facts(), 'routes': {'132.145.111.200': [{'dev': 'lo'}]}}
        private = {**self.netns_facts(), 'routes': {'100.64.1.2': [{'dev': 'ow-w16-n'}]}}
        for facts in (leaky, wrong_route, private, {**self.netns_facts(), 'routes': {}}):
            with self.assertRaises(RuntimeError):
                check_routes(facts, summary)
        # Without declared isolation a bare veth is not a physical uplink.
        with self.assertRaises(RuntimeError):
            check_routes(self.netns_facts())

    def test_namespace_topology_and_worker_wrapping(self):
        topology = self.topology()
        topology['hosts'][1]['netns'] = 'orbit-w16'
        validate_topology(topology)
        for value in ('w16', 'orbit-w16;reboot', 'orbit-$(id)', 'orbit-' + 'x' * 11):
            topology['hosts'][1]['netns'] = value
            with self.assertRaises(RuntimeError):
                validate_topology(topology)
        local = self.topology()
        local['hosts'][0].update(host='local', netns='orbit-w16')
        with self.assertRaises(RuntimeError):
            validate_topology(local)
        n = WANNode('vps', 'VPS', netns='orbit-w16')
        with patch('wan_native.subprocess.run') as command:
            command.return_value = argparse.Namespace(returncode=0, stdout='{}', stderr='')
            n.call('inventory')
            remote = command.call_args.args[0][-1]
            self.assertTrue(remote.startswith('sudo -n ip netns exec orbit-w16 sudo -n -H -u "$(id -un)" python3 -c '))
            self.assertIn('ClearAllForwardings=yes', command.call_args.args[0])

    def test_path_accounting_separates_relay_direct_and_blocked_attempts(self):
        out, inn = account_rules('132.145.111.200', 'out'), account_rules('132.145.111.200', 'in')
        self.assertTrue(out[0].startswith('-o lo') and inn[0].startswith('-i lo'))
        self.assertTrue(all('lan-discovery' in r and 'MULTICAST,BROADCAST' in r for r in (out[1], inn[1])))
        self.assertTrue(all(' -d ' in r or '--dport 53' in r for r in out[2:]))
        self.assertTrue(all(' -s ' in r or '--sport 53' in r for r in inn[2:]))
        self.assertLess(out.index(next(r for r in out if 'service-tcp' in r)),
                        out.index(next(r for r in out if 'direct-tcp' in r)))
        text = '\n'.join([
            '-A w16acct-out -d 132.145.111.200/32 -p tcp -m tcp --dport 8443 -m comment --comment out-service-tcp -c 5 900 -j RETURN',
            '-A w16acct-out ! -d 132.145.111.200/32 -p udp -m comment --comment out-direct-udp -c 9 5000 -j RETURN',
            '-A w16acct-in -s 132.145.111.200/32 -p tcp -m tcp --sport 8443 -m comment --comment in-service-tcp -c 4 700 -j RETURN',
            '-A w16acct-in ! -s 132.145.111.200/32 -p udp -m comment --comment in-direct-udp -c 0 0 -j RETURN'])
        counted = parse_paths(text)
        self.assertEqual(counted['out-direct-udp'], {'packets': 9, 'bytes': 5000})
        # Outbound direct attempts toward a UDP-blocked peer are not a direct path.
        blocked = classify_paths({'Pi': counted})
        self.assertEqual((blocked['direct_bytes'], blocked['direct_attempt_bytes'], blocked['dominant']), (0, 5000, 'relay_or_control'))
        later = parse_paths(text.replace('in-direct-udp -c 0 0', 'in-direct-udp -c 800 1100000'))
        delta = path_delta(counted, later)
        self.assertEqual(delta['in-direct-udp'], {'packets': 800, 'bytes': 1100000})
        self.assertEqual(classify_paths({'VPS': delta})['dominant'], 'direct')
        # Loopback control traffic is never a path; co-located replica UDP is direct.
        self.assertEqual(classify_paths({'Pi': {'in-loopback': {'packets': 9, 'bytes': 99999},
                                                'in-colocated-udp': {'packets': 2, 'bytes': 300}}})['direct_bytes'], 300)

    def test_closed_gate_checked_before_any_host_operation(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            topology = root / 'topology.json'
            topology.write_text(json.dumps(self.topology()))
            args = argparse.Namespace(output=root / 'out', topology=topology, rehearsal_profile=None,
                                      rehearsal_roots=None, inventory_only=False)
            with patch.object(WANNode, 'call') as call, patch('wan_native.check_wg6', side_effect=RuntimeError('WG6 open')):
                with self.assertRaises(RuntimeError):
                    run(args)
                call.assert_not_called()
            report = json.loads((args.output / 'wan-native.json').read_text())
            self.assertFalse(report['success'])
            self.assertEqual(report['hosted_default_acceptance'], 'unexecuted')

    def test_custom_trust_and_wrong_profile_cannot_count_as_hosted_default(self):
        status = {'policy': {'mode': 'automatic', 'profile': 'digest'}, 'ready': True,
                  'service_trust': 'system', 'builtin': {'digest': 'digest'}}
        network_check(status, 'digest', False)
        for update in ({'service_trust': 'custom:abc'}, {'policy': {'mode': 'self_hosted', 'profile': 'digest'}},
                       {'builtin': {'digest': 'other'}}, {'ready': False}):
            with self.assertRaises(RuntimeError):
                network_check(status | update, 'digest', False)

    def test_receipt_is_endpoint_stored_and_applied_is_independently_observed(self):
        from unittest.mock import Mock
        value = b'protected captured bytes'
        head = {'folder': 'folder', 'author': 'owner', 'counter': '1'}
        receipt = {'device': 'receiver', 'version': head, 'stored': True, 'applied': False}
        readiness = {'approved': True, 'membership_current': True, 'scan_complete': True,
                     'missing_content': '0', 'pending_publication': '0', 'conflicts': '0', 'uncaptured': '0'}
        owner, receiver = Mock(device='owner', netns=None, role='Owner'), Mock(device='receiver', netns=None, role='Receiver')
        for node in (owner, receiver):
            node.query.side_effect = lambda kind, **kw: (
                {'versions': [{'digest': hashlib.sha256(value).hexdigest(), 'version': head}]} if kind == 'history'
                else {'observations': [receipt], 'readiness': readiness})
            node.call.return_value = {'sha256': hashlib.sha256(value).hexdigest()}
        def once(label, oracle, seconds):
            result = oracle()
            if not result:
                raise RuntimeError('no acceptance')
            return result
        with patch('wan_native.wait', side_effect=once):
            receiver.netns = 'orbit-w16'
            receiver.isolation.side_effect = [parse_isolation(self.RULES, self.UPLINKS),
                parse_isolation([r.replace('-c 30 3082', '-c 90 70082') for r in self.RULES], self.UPLINKS)]
            routed = transfer([owner, receiver], 'folder', owner, receiver, 'file', value)
            self.assertEqual(routed['route_counters'], {receiver.role: {
                'egress_accept': {'packets': 60, 'bytes': 67000}, 'ingress_established': {'packets': 0, 'bytes': 0},
                'egress_other_rejected': {'packets': 0, 'bytes': 0}, 'nat': {'packets': 0, 'bytes': 0}}})
            receiver.netns = None
            result = transfer([owner, receiver], 'folder', owner, receiver, 'file', value)
            self.assertNotIn('route_counters', result)
            self.assertFalse(result['receipt']['applied'])
            self.assertTrue(result['receiver_working_hash_verified'])
            for key, wrong in [('device', 'broker'), ('stored', False), ('version', {**head, 'counter': '2'})]:
                previous = receipt[key]
                receipt[key] = wrong
                with self.assertRaises(RuntimeError):
                    transfer([owner, receiver], 'folder', owner, receiver, 'file', value)
                receipt[key] = previous
            receiver.call.return_value = {'sha256': 'wrong'}
            with self.assertRaises(RuntimeError):
                transfer([owner, receiver], 'folder', owner, receiver, 'file', value)

    def test_private_certificate_input_and_live_root_removal_guard(self):
        with patch.dict(os.environ), tempfile.TemporaryDirectory(prefix='filesync-validation-', dir=Path.home()) as temp:
            root = Path(temp)
            marker = root / '.filesync-disposable'
            marker.write_text('test-token')
            marker.chmod(0o600)
            req = {'root': str(root), 'token': 'test-token'}
            (root / 'state/identity').mkdir(parents=True)
            pem = b'-----BEGIN CERTIFICATE-----\npublic\n-----END CERTIFICATE-----\n'
            (root / 'state/identity/peer-identity.pem').write_bytes(pem + b'PRIVATE KEY secret')
            with patch('wan_native_agent.subprocess.run') as command:
                command.side_effect = [argparse.Namespace(stdout=b'public-key'), argparse.Namespace(stdout=b'SPKI')]
                result = wan_dispatch({**req, 'action': 'wan-identity'}, dispatch)
                self.assertEqual(result['pin'], hashlib.sha256(b'SPKI').hexdigest())
                self.assertEqual(command.call_args_list[0].kwargs['input'], pem.rstrip(b'\n'))
                self.assertNotIn(b'secret', command.call_args_list[0].kwargs['input'])
            fake_proc = root / '123'
            fake_proc.mkdir()
            (fake_proc / 'cmdline').write_bytes(bytes(root / 'filesync') + b'\0')
            with patch.object(Path, 'iterdir', return_value=iter([fake_proc])):
                with self.assertRaisesRegex(RuntimeError, 'owned process still'):
                    wan_dispatch({**req, 'action': 'wan-clean'}, dispatch)
            self.assertTrue(marker.exists())
            marker.chmod(0o644)
            with self.assertRaisesRegex(RuntimeError, 'marker must be private'):
                wan_dispatch({**req, 'action': 'wan-clean'}, dispatch)


if __name__ == '__main__':
    unittest.main()
