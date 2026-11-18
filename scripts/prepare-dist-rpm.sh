#!/bin/sh
# Stage the pinned Ubuntu query tool outside the locked builder image.
# Run on an Ubuntu 24.04 amd64 build host; never install packages on the host.
set -eu
[ "$#" = 1 ] || { echo 'usage: prepare-dist-rpm.sh SOURCE_ROOT/build-tools' >&2; exit 2; }
. /etc/os-release
[ "$ID:$VERSION_ID:$(uname -m)" = ubuntu:24.04:x86_64 ] || { echo 'Ubuntu 24.04 amd64 tool host required' >&2; exit 2; }
mkdir -p "$1"
root=$(CDPATH= cd -P "$1" && pwd)
mkdir -p "$root/debs" "$root/root" "$root/bin"
cd "$root/debs"
for package in \
 rpm=4.18.2+dfsg-2.1build2 rpm-common=4.18.2+dfsg-2.1build2 \
 librpm9t64=4.18.2+dfsg-2.1build2 librpmio9t64=4.18.2+dfsg-2.1build2 \
 librpmbuild9t64=4.18.2+dfsg-2.1build2 librpmsign9t64=4.18.2+dfsg-2.1build2 \
 liblua5.3-0=5.3.6-2build2 libpopt0=1.19+dfsg-1build1; do
 apt-get download "$package"
done
for file in ./*.deb; do dpkg-deb -x "$file" "$root/root"; done
cat > "$root/bin/rpm" <<'WRAPPER'
#!/bin/sh
set -eu
root=$(CDPATH= cd -P "$(dirname "$0")/../root" && pwd)
export LD_LIBRARY_PATH="$root/usr/lib/x86_64-linux-gnu:$root/lib/x86_64-linux-gnu"
export RPM_CONFIGDIR="$root/usr/lib/rpm"
exec "$root/usr/bin/rpm" "$@"
WRAPPER
chmod 755 "$root/bin/rpm"
"$root/bin/rpm" --version
python3 - "$root" <<'PY'
from pathlib import Path
import hashlib,json,subprocess,sys
root=Path(sys.argv[1]); report={'distribution':'Ubuntu24.04 noble signed APT packages','tool_setup_only':True,'packages':[],'hashes':{}}
for file in sorted((root/'debs').glob('*.deb')):
 report['packages'].append(subprocess.check_output(['dpkg-deb','-f',str(file),'Package','Version'],text=True).strip())
 report['hashes']['debs/'+file.name]=hashlib.sha256(file.read_bytes()).hexdigest()
for name in ('bin/rpm','root/usr/bin/rpm'):
 report['hashes'][name]=hashlib.sha256((root/name).read_bytes()).hexdigest()
(root/'tool-receipt.json').write_text(json.dumps(report,indent=2)+'\n')
PY
