#!/usr/bin/env python3
"""Wrap a verified macOS release archive in an Islander.app disk image."""
import argparse
import hashlib
from pathlib import Path
import platform
import plistlib
import re
import shutil
import subprocess
import tarfile
import tempfile

REPO = Path(__file__).resolve().parents[1]


def sha256(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def build_dmg(archive, version, arch, output):
    if platform.system() != 'Darwin':
        raise RuntimeError('DMG 必须在 macOS 上构建')
    native = {'arm64': 'aarch64'}.get(platform.machine(), platform.machine())
    if native != arch:
        raise RuntimeError('请在对应架构的 macOS 上构建并验证启动')
    destination = output / f'islander-{version}-macos-{arch}.dmg'
    if destination.exists():
        raise RuntimeError(f'产物已存在，拒绝覆盖：{destination}')
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='islander-dmg-') as directory:
        work = Path(directory)
        content = work / 'content'
        app = content / 'Islander.app'
        contents = app / 'Contents'
        resources = contents / 'Resources'
        executables = contents / 'MacOS'
        resources.mkdir(parents=True)
        executables.mkdir()
        with tarfile.open(archive) as bundle:
            if sorted(bundle.getnames()) != ['README.md', 'islander']:
                raise RuntimeError('发布包文件清单不符')
            member = bundle.getmember('islander')
            if not member.isfile() or member.size > 100 * 1024 * 1024:
                raise RuntimeError('发布包可执行文件格式异常')
            with bundle.extractfile(member) as source, (resources / 'islander').open('wb') as target:
                shutil.copyfileobj(source, target)
        binary = resources / 'islander'
        binary.chmod(0o755)
        actual = subprocess.check_output([str(binary), '--version'], text=True).strip()
        if actual != 'islander version ' + version:
            raise RuntimeError('发布包版本与 DMG 版本不符：' + actual)
        expected_arch = 'arm64' if arch == 'aarch64' else 'x86_64'
        subprocess.run(['lipo', str(binary), '-verify_arch', expected_arch], check=True)
        launcher = executables / 'islander-launcher'
        shutil.copy2(REPO / 'packaging/macos-launcher', launcher)
        launcher.chmod(0o755)
        subprocess.run(['/bin/sh', '-n', str(launcher)], check=True)
        with (contents / 'Info.plist').open('wb') as stream:
            plistlib.dump({
                'CFBundleExecutable': launcher.name,
                'CFBundleIdentifier': 'top.islander.terminal',
                'CFBundleName': 'Islander',
                'CFBundleDisplayName': 'Islander',
                'CFBundlePackageType': 'APPL',
                'CFBundleShortVersionString': version.removeprefix('v').split('-')[0],
                'CFBundleVersion': version.removeprefix('v').split('-')[0],
                'LSUIElement': True,
                'NSAppleEventsUsageDescription': '在 Terminal 中打开 Islander 论坛终端界面。',
            }, stream)
        subprocess.run(['plutil', '-lint', str(contents / 'Info.plist')], check=True)
        # Ad-hoc bundle seal; this is not Developer ID signing or notarization.
        subprocess.run(['codesign', '--force', '--sign', '-', str(app)], check=True)
        subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
        (content / 'Applications').symlink_to('/Applications')
        (content / '使用说明.txt').write_text(
            f'Islander {version}\n\n'
            '将 Islander.app 拖到 Applications，然后双击打开。\n'
            '启动器会在系统 Terminal 中运行 TUI；首次启动可能需要允许控制 Terminal。\n'
            '使用 Ghostty：将 Islander.app/Contents/Resources/islander 拖入终端，添加 tui 参数运行。\n'
            '此包未做 Apple Developer ID 签名或公证。\n', encoding='utf-8')
        image = work / destination.name
        subprocess.run(['hdiutil', 'create', '-volname', f'Islander {version}',
                        '-srcfolder', str(content), '-format', 'UDZO', str(image)], check=True)
        subprocess.run(['hdiutil', 'verify', str(image)], check=True)
        mount = work / 'mounted'
        subprocess.run(['hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', str(mount), str(image)], check=True)
        try:
            installed = mount / 'Islander.app/Contents/MacOS/islander-launcher'
            actual = subprocess.check_output([str(installed), '--version'], text=True).strip()
            if actual != 'islander version ' + version:
                raise RuntimeError('DMG 中启动器版本检查失败')
            subprocess.run([str(installed), '--help'], stdout=subprocess.DEVNULL, check=True)
        finally:
            subprocess.run(['hdiutil', 'detach', str(mount)], check=True)
        shutil.move(image, destination)
    print(f'{sha256(destination)}  {destination.name}', flush=True)
    return destination


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--arch', choices=('x86_64', 'aarch64'), required=True)
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--checksums', type=Path, required=True)
    parser.add_argument('--output', type=Path, default=REPO / 'dist')
    args = parser.parse_args()
    if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?', args.version):
        parser.error('版本必须形如 v0.0.2')
    expected_name = f'islander-{args.version}-macos-{args.arch}.tar.gz'
    checks = {name: value for value, name in (line.split() for line in args.checksums.read_text().splitlines())}
    if args.archive.name != expected_name or sha256(args.archive) != checks.get(expected_name):
        parser.error('原始发布包 SHA-256 或名称不符')
    build_dmg(args.archive, args.version, args.arch, args.output)


if __name__ == '__main__':
    main()
