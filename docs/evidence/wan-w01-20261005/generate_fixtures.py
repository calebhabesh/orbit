from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization, hashes
from cryptography import x509
from cryptography.x509.oid import NameOID
import datetime,hashlib,json,struct,pathlib,base64
root=pathlib.Path('schemas/fixtures/network-v1');root.mkdir(parents=True,exist_ok=True)
def hx(n):return (bytes([n])*32).hex()
def can(kind,fields):return b''.join(struct.pack('>I',len(s.encode()))+s.encode() for s in ['orbit-network-v1',kind]+fields)
def digest(b):return hashlib.sha256(b).hexdigest()
def save(name,obj,canonical,key):
 obj['signature']=key.sign(canonical).hex()
 (root/(name+'.json')).write_text(json.dumps(obj,indent=2)+'\n')
 (root/(name+'.hex')).write_text(canonical.hex()+'\n')
key=Ed25519PrivateKey.from_private_bytes(bytes([1])*32)
pub=key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
cert=x509.CertificateBuilder().subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'synthetic-w01')])).issuer_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'synthetic-w01')])).public_key(key.public_key()).serial_number(1).not_valid_before(datetime.datetime(2026,1,1)).not_valid_after(datetime.datetime(2030,1,1)).sign(key,None)
der=base64.b64encode(cert.public_bytes(serialization.Encoding.DER)).decode()
pin=digest(key.public_key().public_bytes(serialization.Encoding.DER,serialization.PublicFormat.SubjectPublicKeyInfo))
online=Ed25519PrivateKey.from_private_bytes(bytes([16])*32)
servicekey=online.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
p={'service_key':servicekey,'version':'1','operator':'Orbit synthetic test operator','authority':pub,'epoch':'1','expires':'1800000600','origins':['https://rendezvous.example.org','wss://relay.example.org'],'stun':['8.8.8.8:3478'],'privacy':'Synthetic fixture: addresses, identities and timing visible; no production service.'}
pc=can('profile',[p['version'],p['operator'],p['authority'],p['service_key'],p['epoch'],p['expires'],str(len(p['origins']))]+p['origins']+[str(len(p['stun']))]+p['stun']+[p['privacy']]);save('profile',p,pc,key);profile=digest(pc)
a={'generation':'1','expires':'1800000600','candidates':[{'transport':'tcp','address':'8.8.8.8:7443','scope':'public','interface':''}],'capabilities':['direct_https_v1','relay_inner_tls_v1'],'relay':True}
ac=can('announcement',[a['generation'],a['expires'],'1','tcp','8.8.8.8:7443','public','','2']+a['capabilities']+['true']);(root/'announcement.json').write_text(json.dumps(a,indent=2)+'\n');(root/'announcement.hex').write_text(ac.hex()+'\n')
q={'version':'1','kind':'announce','profile':profile,'origin':p['origins'][0],'sender':hx(2),'sender_pin':pin,'target':'','target_pin':'','purpose':'peer_data','challenge':hx(3),'operation':hx(4),'session':'','generation':'1','role':'','expires':'1800000060','payload':digest(ac),'certificate_der':der}
qc=can(q['kind'],[q[n] for n in ['version','profile','origin','sender','sender_pin','target','target_pin','purpose','challenge','operation','session','generation','role','expires','payload']]);save('proof',q,qc,key)
t={'profile':profile,'origin':p['origins'][1],'epoch':hx(5),'session':hx(6),'device':hx(2),'pin':pin,'partner':hx(7),'partner_pin':hx(8),'purpose':'enrollment','role':'initiator','expires':'1800000030'}
tc=can('relay-attachment',[t[n] for n in ['profile','origin','epoch','session','device','pin','partner','partner_pin','purpose','role','expires']]);save('attachment',t,tc,online)
route={'device':hx(7),'pin':pin,'profile':profile,'purpose':'enrollment'}
requester={'device':hx(2),'pin':pin,'profile':profile,'purpose':'enrollment'}
cap=hx(9)
e={'version':'3','folder':hx(10),'inviter':route,'requester':requester,'capability_digest':digest(bytes.fromhex(cap)),'attempt':hx(11),'challenge':hx(12),'public_key':pub,'prior_membership':hx(13),'expires':'1800000600','label':'Synthetic laptop'}
ec=can('enrollment-request-v3',[e['version'],e['folder']]+list(route.values())+list(requester.values())+[e[n] for n in ['capability_digest','attempt','challenge','public_key','prior_membership','expires','label']])
w={'transcript':e,'capability':cap,'certificate_der':der};save('enrollment-request',w,ec,key)
req=digest(can('enrollment-attempt-v3',[e['folder'],route['device'],route['pin'],requester['device'],requester['pin'],e['attempt'],profile]))
s={'version':'3','request':req,'transcript_digest':digest(ec),'requester':requester,'nonce':hx(14),'expires':'1800000060'}
sc=can('enrollment-status-v3',[s['version'],s['request'],s['transcript_digest']]+list(requester.values())+[s['nonce'],s['expires']]);save('enrollment-status',s,sc,key)
ap={'version':'3','request':req,'transcript_digest':digest(ec),'prior_membership':hx(13),'membership_digest':hx(15)}
apc=can('enrollment-approval-v3',list(ap.values()));(root/'enrollment-approval.json').write_text(json.dumps(ap,indent=2)+'\n');(root/'enrollment-approval.hex').write_text(apc.hex()+'\n')
inv={'version':'3','folder':e['folder'],'inviter_route':route,'certificate_der':der,'capability':cap,'expires':'1800000600','profile':p}
(root/'invitation.json').write_text(json.dumps(inv,indent=2)+'\n')
(root/'README.md').write_text('Synthetic independent fixtures. Python cryptography Ed25519 signer, device/profile seed 01 and online relay seed 10 (hex), each repeated 32 bytes, fixed certificate/time. Never production credentials. Canonical framing uses explicit ordered fields, not Go encoder. Generator retained in W01 evidence. Verification time 1800000000.\n')
# Boundary goldens, still independent of the Go implementation.
maximum=dict(p);maximum.update(operator='o'*128,privacy='p'*4096,epoch='18446744073709551615',expires='18446744073709551615')
# Four distinct canonical origins, each exactly 256 bytes.
maximum['origins']=['https://'+('a'*63)+'.'+('b'*63)+'.'+('c'*63)+'.'+str(i)+('d'*55) for i in range(4)]
assert all(len(s)==256 for s in maximum['origins'])
maximum['stun']=['8.8.8.8:'+str(65535-i) for i in range(4)]
mc=can('profile',[maximum['version'],maximum['operator'],maximum['authority'],maximum['service_key'],maximum['epoch'],maximum['expires'],'4']+maximum['origins']+['4']+maximum['stun']+[maximum['privacy']]);save('maximum-profile',maximum,mc,key)
ma={'generation':'18446744073709551615','expires':'1800000600','candidates':[{'transport':'tcp','address':'8.8.8.8:'+str(65535-i),'scope':'public','interface':''} for i in range(16)],'capabilities':['direct_https_v1','relay_inner_tls_v1','enrollment_v3','quic_ice_v1'],'relay':True}
mac=can('announcement',[ma['generation'],ma['expires'],'16']+[s for c in ma['candidates'] for s in c.values()]+['4']+ma['capabilities']+['true']);(root/'maximum-announcement.json').write_text(json.dumps(ma,indent=2)+'\n');(root/'maximum-announcement.hex').write_text(mac.hex()+'\n')
for name,obj in [('profile',dict(maximum,privacy=maximum['privacy']+'x')),('announcement',dict(ma,candidates=ma['candidates']+[{'transport':'udp','address':'8.8.8.8:1','scope':'public','interface':''}]))]:
 (root/('over-limit-'+name+'.json')).write_text(json.dumps(obj,indent=2)+'\n')
lookup=dict(q,kind='lookup',target=hx(7),target_pin=hx(8),generation='0')
intent=can('lookup-intent',[lookup[n] for n in ['profile','origin','sender','sender_pin','target','target_pin','purpose','operation','session','generation','role']])
lookup['payload']=digest(intent)
lookupcanonical=can('lookup',[lookup[n] for n in ['version','profile','origin','sender','sender_pin','target','target_pin','purpose','challenge','operation','session','generation','role','expires','payload']]);save('lookup-proof',lookup,lookupcanonical,key)
(root/'lookup-intent.hex').write_text(intent.hex()+'\n')
