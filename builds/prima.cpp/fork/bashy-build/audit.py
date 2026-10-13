#!/usr/bin/env python3
"""Turn reviewed inventory + compiled file closure into SBOM and gate reports."""
import hashlib
import json
from pathlib import Path
import re
import sys
import urllib.request

root=Path(__file__).resolve().parent
out=Path('out')
inventory=json.loads((root/'dependencies.json').read_text())
scan=json.loads((out/'prima-licenses.json').read_text())
allowed={'mit','bsd-new','bsd-simplified','bsd-original','apache-2.0','isc','public-domain'}
errors=[]
inputs=json.loads((out/'prima-inputs.json').read_text())
expected={f['path'] for f in inputs}
observed=set()
for f in scan['files']:
    if f.get('type') == 'file':
        path=f['path']
        if path.startswith('audit-inputs/'): path=path[len('audit-inputs/'):]
        observed.add(path)
if not expected or observed != expected:
    raise ValueError('ScanCode must cover every compiler input, on all four platforms')
for f in scan['files']:
    if f.get('scan_errors'): errors.append(f"scan error: {f['path']}")
    for detection in f.get('license_detections',[]):
        expression=detection['license_expression']
        ids=set(re.findall(r'[a-z][a-z0-9.+-]*',expression))-{'and','or','with'}
        if not ids <= allowed: errors.append(f"unreviewed license {expression}: {f['path']}")
# Headers without their own license inherit the pinned root license; preserve all findings.
components=[]
for dep in inventory['linked']:
    components.append({'type':'library','name':dep['name'],'version':dep['version'],
                       'purl':dep['purl'],'licenses':[{'license':{'id':dep['license']}}]})
for f in inputs:
    components.append({'type':'file','name':f['path'],'hashes':[{'alg':'SHA-256','content':f['sha256']}]})
bom={'bomFormat':'CycloneDX','specVersion':'1.6','version':1,'components':components}
(out/'prima-sbom.cdx.json').write_text(json.dumps(bom,indent=2)+'\n')
# Query exact git commits, including the vendored JSON version via its upstream tag.
queries=[{'commit':inventory['source']['commit']},{'commit':inventory['linked'][1]['commit']},
         {'package':{'name':'nlohmann-json','ecosystem':'OSS-Fuzz'},'version':'3.11.3'},
         {'package':{'name':'llama.cpp','ecosystem':'OSS-Fuzz'},'version':inventory['source']['commit']}]
req=urllib.request.Request('https://api.osv.dev/v1/querybatch',data=json.dumps({'queries':queries}).encode(),headers={'Content-Type':'application/json'})
with urllib.request.urlopen(req,timeout=60) as r: result=json.load(r)
if len(result['results'])!=len(queries) or any(not isinstance(r,dict) or 'error' in r for r in result['results']):
    raise ValueError('incomplete or failed OSV response')
(out/'prima-osv.json').write_text(json.dumps({'queries':queries,'response':result},indent=2)+'\n')
if any(r.get('vulns') for r in result['results']): errors.append('OSV vulnerabilities require review/remediation (no waivers)')
if errors:
    print('\n'.join(errors),file=sys.stderr)
    sys.exit(1)
print('PASS: compiled-source license gate and exact-query OSV gate; see inventory audit limits')
