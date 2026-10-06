#!/usr/bin/env python3
"""Independent canonical/signature fixture; all credentials are synthetic."""
import hashlib
import json
from pathlib import Path
import struct
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

root = Path(__file__).resolve().parent

def canonical(kind, *fields):
    values = [b'orbit-network-v1', kind.encode(), *[v if isinstance(v, bytes) else v.encode() for v in fields]]
    return b''.join(struct.pack('>I', len(v)) + v for v in values)

ice = dict(mode='offer', ufrag='abcd', password='a'*32,
           candidates=['1 1 udp 2130706431 11.23.45.1 12345 typ host'])
inner = canonical('ice-v1', ice['mode'], ice['ufrag'], ice['password'], '1', *ice['candidates'])
payload = canonical('ice-offer-v1', '1', '2', '0', inner)
proof = json.loads((root.parent/'network-v1/proof.json').read_text())
proof.update(kind='offer', target='05'*32, target_pin='06'*32, session='07'*32,
             role='initiator', payload=hashlib.sha256(payload).hexdigest())
fields = ['version', 'profile', 'origin', 'sender', 'sender_pin', 'target',
          'target_pin', 'purpose', 'challenge', 'operation', 'session', 'generation',
          'role', 'expires', 'payload']
message = canonical('offer', *[proof[f] for f in fields])
proof['signature'] = Ed25519PrivateKey.from_private_bytes(bytes([1])*32).sign(message).hex()
(root/'offer.json').write_text(json.dumps(dict(proof=proof, offer=dict(sender_generation='1', target_generation='2', candidates=[], ice=ice)), indent=2)+'\n')
(root/'offer.hex').write_text(message.hex()+'\n')
(root/'payload.hex').write_text(payload.hex()+'\n')
