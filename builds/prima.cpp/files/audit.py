#!/usr/bin/env python3
"""Turn reviewed inventory + compiled file closure into SBOM and gate reports."""
import json
from pathlib import Path
import runpy
import sys
import urllib.request

root=Path(__file__).resolve().parent
out=Path('out')
inventory=json.loads((root/'dependencies.json').read_text())
scan=json.loads((out/'prima-licenses.json').read_text())
inputs=json.loads((out/'prima-inputs.json').read_text())
# Load the adjacent hyphenated module without changing the process import path.
review=runpy.run_path(str(root/'license-review.py'))
errors, corrections=review['audit_licenses'](scan, inputs, Path('audit-inputs'), inventory)
scan['bashy_reviewed_findings']=corrections
(out/'prima-licenses.json').write_text(json.dumps(scan,indent=2)+'\n')
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
