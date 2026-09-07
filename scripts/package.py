#!/usr/bin/env python3
"""Build versioned Linux archives and AppImages from an explicit file list."""
import argparse
import hashlib
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request

REPO = Path(__file__).resolve().parents[1]
ARCHES = {'x86_64': 'amd64', 'aarch64': 'arm64'}
TOOL_VERSION = '1.9.1'
TOOL_HASHES = {
    'x86_64': 'ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0',
    'aarch64': 'f0837e7448a0c1e4e650a93bb3e85802546e60654ef287576f46c71c126a9158',
}
# Official type2-runtime continuous assets, reviewed 2026-09-07.
# If upstream replaces them, review the new release and update these hashes.
RUNTIME_HASHES = {
    'x86_64': '1cc49bcf1e2ccd593c379adb17c9f85a36d619088296504de95b1d06215aebbf',
    'aarch64': '7d5d772b7c32f0c84caf0a452a3072a5709027d7eac5856feb89a7a7a8881372',
}


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def download(url, expected, target):
    if target.exists() and digest(target) == expected:
        return target
    target.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(dir=target.parent)
    temp = Path(name)
    try:
        with os.fdopen(fd, 'wb') as out:
            request = urllib.request.Request(url, headers={'User-Agent': 'islander-build'})
            with urllib.request.urlopen(request, timeout=60) as response:
                shutil.copyfileobj(response, out)
        if digest(temp) != expected:
            raise RuntimeError(f'校验失败：{target.name}，请核对上游版本；未执行下载文件')
        temp.chmod(0o755)
        temp.replace(target)
    finally:
        temp.unlink(missing_ok=True)
    return target


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True, help='例如 v0.0.1')
    parser.add_argument('--arch', choices=ARCHES, default=platform.machine())
    parser.add_argument('--appimage', action='store_true', help='同时构建 AppImage，需要本机架构 Linux')
    args = parser.parse_args()
    if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?', args.version):
        parser.error('发布版本必须形如 v0.0.1 或 v0.0.1-rc.1')
    if args.arch not in ARCHES:
        parser.error('请指定 --arch x86_64 或 aarch64')
    if args.appimage and (platform.system() != 'Linux' or platform.machine() != args.arch):
        parser.error('AppImage 必须在对应架构的 Linux 上构建')
    if args.appimage and not shutil.which('desktop-file-validate'):
        parser.error('AppImage 构建需要 desktop-file-validate（Debian/Ubuntu: desktop-file-utils）')
    output = REPO / 'dist'
    output.mkdir(exist_ok=True)
    build_root = REPO / '.local/release-build'
    build_root.mkdir(parents=True, exist_ok=True)
    name = f'islander-{args.version}-linux-{args.arch}'
    outputs = [output / (name + '.tar.gz')]
    if args.appimage:
        outputs.append(output / (name + '.AppImage'))
    if any(p.exists() for p in outputs):
        parser.error('目标产物已存在；请先移动旧文件，避免覆盖已发布版本')
    env = dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH=ARCHES[args.arch])
    with tempfile.TemporaryDirectory(prefix=name + '-', dir=build_root) as directory:
        work = Path(directory)
        binary = work / 'islander'
        subprocess.run(['go', 'build', '-trimpath', '-ldflags',
                        f'-s -w -X github.com/A-islander/islander-cli/internal/cli.Version={args.version}',
                        '-o', str(binary), './cmd/islander'], cwd=REPO, env=env, check=True)
        if platform.system() == 'Linux' and platform.machine() == args.arch:
            actual = subprocess.check_output([str(binary), '--version'], text=True).strip()
            if actual != 'islander version ' + args.version:
                raise RuntimeError('可执行文件版本与发布版本不符：' + actual)
            subprocess.run([str(binary), '--help'], stdout=subprocess.DEVNULL, check=True)
        archive = work / outputs[0].name
        with tarfile.open(archive, 'w:gz') as bundle:
            bundle.add(binary, arcname='islander')
            bundle.add(REPO / 'README.md', arcname='README.md')
        if args.appimage:
            appdir = work / 'Islander.AppDir'
            (appdir / 'usr/bin').mkdir(parents=True)
            shutil.copy2(binary, appdir / 'usr/bin/islander')
            for resource in ('AppRun', 'islander.desktop', 'islander.svg'):
                shutil.copy2(REPO / 'packaging' / resource, appdir / resource)
            (appdir / 'AppRun').chmod(0o755)
            (appdir / '.DirIcon').symlink_to('islander.svg')
            subprocess.run(['desktop-file-validate', str(appdir / 'islander.desktop')], check=True)
            cache = REPO / '.cache/appimage-tools'
            tool = download(
                f'https://github.com/AppImage/appimagetool/releases/download/{TOOL_VERSION}/appimagetool-{args.arch}.AppImage',
                TOOL_HASHES[args.arch], cache / f'appimagetool-{TOOL_VERSION}-{args.arch}')
            runtime = download(
                f'https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-{args.arch}',
                RUNTIME_HASHES[args.arch], cache / f'runtime-{args.arch}-{RUNTIME_HASHES[args.arch][:12]}')
            image = work / outputs[1].name
            subprocess.run([str(tool), '--appimage-extract-and-run', '--runtime-file', str(runtime),
                            '--no-appstream', str(appdir), str(image)],
                           env=dict(env, ARCH=args.arch, VERSION=args.version), check=True)
            actual = subprocess.check_output([str(image), '--appimage-extract-and-run', '--version'], text=True).strip()
            if actual != 'islander version ' + args.version:
                raise RuntimeError('AppImage 版本检查失败：' + actual)
            subprocess.run([str(image), '--appimage-extract-and-run', '--help'],
                           stdout=subprocess.DEVNULL, check=True)
        for destination in outputs:
            (work / destination.name).replace(destination)
            print(f'{digest(destination)}  {destination.name}', flush=True)


if __name__ == '__main__':
    main()
