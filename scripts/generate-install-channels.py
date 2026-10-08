#!/usr/bin/env python3
"""Generate thin channel manifests from already verified product archives.

No installer logic and no rebuild. Output stays local for channel validation
and publication; repository creation and credentials are release prerequisites.
"""
import argparse
import hashlib
import json
import pathlib
import re

parser = argparse.ArgumentParser()
parser.add_argument('tag')
parser.add_argument('dist', type=pathlib.Path)
parser.add_argument('output', type=pathlib.Path)
a = parser.parse_args()
if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-dev)?', a.tag):
    parser.error('expected vX.Y.Z or vX.Y.Z-dev')
version = a.tag[1:]
base = f'https://github.com/qiangli/bashy/releases/download/{a.tag}/'
sums = {}
for line in (a.dist / 'checksums.txt').read_text().splitlines():
    digest, name = line.split('  ', 1)
    if name in sums or not re.fullmatch(r'[a-f0-9]{64}', digest):
        raise SystemExit('invalid or duplicate checksum entry')
    sums[name] = digest

def artifact(os_name, arch):
    name = f'bashy-{os_name}-{arch}.' + ('zip' if os_name == 'windows' else 'tar.gz')
    if hashlib.sha256((a.dist / name).read_bytes()).hexdigest() != sums.get(name):
        raise SystemExit(f'archive checksum mismatch: {name}')
    return base + name, sums[name]

out = a.output
(out / 'Formula').mkdir(parents=True, exist_ok=True)
(out / 'bucket').mkdir(parents=True, exist_ok=True)
# A formula (not a cask) supplies brew services' service DSL.
formula = ['class Bashy < Formula', '  desc "Bashy userland and outpost service"',
           '  homepage "https://github.com/qiangli/bashy"',
           f'  version "{version}"', '  license all_of: ["BSD-3-Clause", "MIT"]']
for os_name, block in [('darwin', 'on_macos'), ('linux', 'on_linux')]:
    formula.append(f'  {block} do')
    for arch, cpu in [('arm64', 'on_arm'), ('amd64', 'on_intel')]:
        url, sha = artifact(os_name, arch)
        formula += [f'    {cpu} do', f'      url "{url}"', f'      sha256 "{sha}"', '    end']
    formula.append('  end')
formula += ['  def install', '    bin.install "bashy", "outpost"',
            '    # bash and sh stay off PATH so they never shadow Homebrew bash or /bin/sh.',
            '    libexec.install "bash", "sh"', '  end',
            '  service do', '    run [opt_bin/"outpost", "supervisord"]', '    keep_alive true', '  end',
            '  test do', '    require "json"',
            '    shell = shell_output("#{bin}/bashy --version 2>&1")[/bashy-(v?[0-9]+\\.[0-9]+\\.[0-9]+(?:-dev)?)/, 1]',
            '    service = JSON.parse(shell_output("#{bin}/outpost version --json")).fetch("version")',
            '    refute_nil shell',
            '    assert_equal shell.delete_prefix("v").delete_suffix("-dev"), service.delete_prefix("v").delete_suffix("-dev")',
            '  end', 'end', '']
(out / 'Formula/bashy.rb').write_text('\n'.join(formula))
scoop = {'version': version, 'description': 'Bashy userland and outpost service',
         'homepage': 'https://github.com/qiangli/bashy', 'license': {'identifier': 'BSD-3-Clause AND MIT'},
         'architecture': {}, 'bin': ['bashy.exe', 'outpost.exe', 'bash.exe', 'sh.exe']}
for arch, scoop_arch in [('amd64', '64bit'), ('arm64', 'arm64')]:
    url, sha = artifact('windows', arch)
    scoop['architecture'][scoop_arch] = {'url': url, 'hash': sha}
(out / 'bucket/bashy.json').write_text(json.dumps(scoop, indent=2) + '\n')
# Winget's singleton schema caps Installers at one entry. The portable package
# ships both x64 and arm64, so emit the normal three-file manifest set.
winget_dir = out / 'winget'
winget_dir.mkdir(parents=True, exist_ok=True)
common = ['PackageIdentifier: qiangli.bashy', f'PackageVersion: {json.dumps(version)}']
(winget_dir / 'qiangli.bashy.yaml').write_text('\n'.join([
    '# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.1.6.0.schema.json',
    *common, 'DefaultLocale: en-US', 'ManifestType: version', 'ManifestVersion: 1.6.0', '']))
(winget_dir / 'qiangli.bashy.locale.en-US.yaml').write_text('\n'.join([
    '# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.1.6.0.schema.json',
    *common, 'PackageLocale: en-US', 'Publisher: Qiang Li', 'PackageName: bashy',
    'License: BSD-3-Clause AND MIT', 'ShortDescription: Bashy userland and outpost service',
    'ManifestType: defaultLocale', 'ManifestVersion: 1.6.0', '']))
winget = ['# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.1.6.0.schema.json',
          *common, 'InstallerType: zip', 'NestedInstallerType: portable', 'NestedInstallerFiles:']
for name in ['bashy', 'outpost', 'bash', 'sh']:
    winget += [f'  - RelativeFilePath: {name}.exe', f'    PortableCommandAlias: {name}']
winget.append('Installers:')
for arch, winget_arch in [('amd64', 'x64'), ('arm64', 'arm64')]:
    url, sha = artifact('windows', arch)
    winget += [f'  - Architecture: {winget_arch}', f'    InstallerUrl: {url}', f'    InstallerSha256: {sha.upper()}']
winget += ['ManifestType: installer', 'ManifestVersion: 1.6.0', '']
(winget_dir / 'qiangli.bashy.installer.yaml').write_text('\n'.join(winget))
print(f'Generated Homebrew, Scoop and Winget manifests at {out}; publication and native installation remain separate gates.')
