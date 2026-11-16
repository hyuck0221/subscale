#!/usr/bin/env python3
"""Check binary installation and checksum rejection without network or host changes."""
import hashlib
import io
import os
from pathlib import Path
import platform
import subprocess
import tarfile
import tempfile

installer = Path(__file__).resolve().with_name('install.sh')
system = {'Darwin': 'darwin', 'Linux': 'linux'}[platform.system()]
arch = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64', 'AMD64': 'amd64'}[platform.machine()]
archive_name = f'subscale_{system}_{arch}.tar.gz'
with tempfile.TemporaryDirectory(prefix='subscale-installer-') as root:
    root = Path(root)
    fixture = root / 'release'
    fixture.mkdir()
    archive = fixture / archive_name
    with tarfile.open(archive, 'w:gz') as tar:
        data = b'#!/bin/sh\nprintf "subscale fixture\\n"\n'
        info = tarfile.TarInfo('subscale')
        info.mode = 0o755
        info.size = len(data)
        tar.addfile(info, io.BytesIO(data))
    manifest = fixture / 'checksums.txt'
    checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
    manifest.write_text(f'{checksum}  {archive_name}\n')
    tools = root / 'tools'
    tools.mkdir()
    curl = tools / 'curl'
    curl.write_text('''#!/usr/bin/env python3
import os, sys, shutil
from pathlib import Path
args = sys.argv[1:]
url = next(a for a in args if a.startswith('https://'))
output = args[args.index('-o') + 1]
fixture = Path(os.environ['INSTALLER_FIXTURE'])
with (fixture / 'requests.txt').open('a') as log:
    log.write(url + '\\n')
shutil.copyfile(fixture / url.rsplit('/', 1)[1], output)
''')
    curl.chmod(0o755)
    dest = root / 'custom bin'
    env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ['PATH'],
               SUBSCALE_INSTALL_DIR=str(dest), INSTALLER_FIXTURE=str(fixture))
    env.pop('SUBSCALE_VERSION', None)

    def install(success=True):
        result = subprocess.run(['sh', str(installer)], env=env, capture_output=True, timeout=20)
        assert (result.returncode == 0) == success, result.stderr.decode()
        return result

    install()
    binary = dest / 'subscale'
    assert subprocess.check_output([str(binary)]) == b'subscale fixture\n'
    assert binary.stat().st_mode & 0o111
    assert '/releases/latest/download/' in (fixture / 'requests.txt').read_text()
    original = binary.read_bytes()
    manifest.write_text(f'{"0" * 64}  {archive_name}\n')
    assert b'Checksum verification failed' in install(False).stderr
    assert binary.read_bytes() == original
    manifest.write_text('')
    assert b'checksum is missing' in install(False).stderr
    assert binary.read_bytes() == original
    manifest.write_text(f'{checksum}  {archive_name}\n')
    env['SUBSCALE_VERSION'] = 'v0.1.0'
    install()
    assert '/releases/download/v0.1.0/' in (fixture / 'requests.txt').read_text()
    assert not list(dest.glob('.subscale.*'))
    print('PASS: binary install, executable permissions, latest/pinned URLs, checksum rejection, atomic replacement')
