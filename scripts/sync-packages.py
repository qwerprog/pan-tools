#!/usr/bin/env python3
"""Update Homebrew, WinGet, and AUR package manifests from main.go and GitHub Release digests."""
import argparse
from datetime import date
import json
import re
import shutil
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
REPO_SLUG = "qwerprog/pan-tools"

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument(
    "--version",
    required=False,
    help="Release version without leading 'v' (defaults to parsing main.go)",
)
parser.add_argument(
    "--checksums",
    type=Path,
    required=False,
    help="Optional local SHA256SUMS file; defaults to reading digests via `gh release view`",
)
args = parser.parse_args()

if args.version:
    version = args.version.lstrip("v")
else:
    main_go = (ROOT / "main.go").read_text(encoding="utf-8")
    m = re.search(r'Version\s*=\s*"v?([0-9]+\.[0-9]+\.[0-9]+)(?:-[^"]*)?"', main_go)
    if not m:
        raise ValueError("Unable to parse Version from main.go")
    version = m.group(1)

hashes: dict[str, str] = {}
if args.checksums:
    for line in args.checksums.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        digest, name = line.split()
        if not re.fullmatch(r"[a-fA-F0-9]{64}", digest):
            raise ValueError(f"Invalid SHA256 for {name}")
        hashes[name.lstrip("*")] = digest.lower()
else:
    raw = subprocess.check_output(
        ["gh", "release", "view", f"v{version}", "--repo", REPO_SLUG, "--json", "assets"],
        cwd=ROOT,
        text=True,
    )
    for asset in json.loads(raw).get("assets", []):
        digest = asset.get("digest") or ""
        if digest.startswith("sha256:"):
            hex_digest = digest.removeprefix("sha256:").lower()
            if re.fullmatch(r"[a-f0-9]{64}", hex_digest):
                hashes[asset["name"]] = hex_digest

base = f"https://github.com/{REPO_SLUG}/releases/download/v{version}"
for platform, ext in [("windows", "zip"), ("darwin", "zip"), ("linux", "tar.gz")]:
    for arch in ["amd64", "arm64"]:
        pkg_name = f"pant-v{version}-{platform}-{arch}.{ext}"
        if pkg_name not in hashes:
            raise ValueError(f"Missing {pkg_name} checksum")

# 1. Homebrew Formula
brew = ROOT / "pkg/homebrew/Formula/pant.rb"
brew.parent.mkdir(parents=True, exist_ok=True)
brew.write_text(
    f'''class Pant < Formula
  desc "Modern Baidu Netdisk CLI & interactive TUI client"
  homepage "https://github.com/{REPO_SLUG}"
  version "{version}"
  license "Apache-2.0"

  on_intel do
    url "{base}/pant-v{version}-darwin-amd64.zip"
    sha256 "{hashes[f'pant-v{version}-darwin-amd64.zip']}"
  end

  on_arm do
    url "{base}/pant-v{version}-darwin-arm64.zip"
    sha256 "{hashes[f'pant-v{version}-darwin-arm64.zip']}"
  end

  def install
    bin.install "pant"
    doc.install "README.md"
    prefix.install "LICENSE", "NOTICE"
  end

  test do
    assert_match "{version}", shell_output("#{{bin}}/pant --version")
  end
end
''',
    encoding="utf-8",
)

# 2. WinGet Manifests (Schema 1.12.0)
today = date.today().isoformat()
installer_entries = "Installers:\n"
for wg_arch, go_arch in [("x64", "amd64"), ("arm64", "arm64")]:
    filename = f"pant-v{version}-windows-{go_arch}.zip"
    rel_exe = f"pant-v{version}-windows-{go_arch}\\pant.exe"
    installer_entries += f"""  - Architecture: {wg_arch}
    InstallerUrl: {base}/{filename}
    InstallerSha256: {hashes[filename].upper()}
    NestedInstallerFiles:
      - RelativeFilePath: {rel_exe}
        PortableCommandAlias: pant
"""

installer_manifest = f"""# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.1.12.0.schema.json

PackageIdentifier: QwerProg.pan-tools
PackageVersion: {version}
InstallerLocale: en-US
InstallerType: zip
NestedInstallerType: portable
ReleaseDate: {today}
{installer_entries}ManifestType: installer
ManifestVersion: 1.12.0
"""

locale_manifest = f"""# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.1.12.0.schema.json

PackageIdentifier: QwerProg.pan-tools
PackageVersion: {version}
PackageLocale: en-US
Publisher: QwerProg
PublisherUrl: https://github.com/{REPO_SLUG}
PublisherSupportUrl: https://github.com/{REPO_SLUG}/issues
PackageName: pan-tools
PackageUrl: https://github.com/{REPO_SLUG}
License: Apache-2.0
LicenseUrl: https://github.com/{REPO_SLUG}/blob/main/LICENSE
ShortDescription: Modern Baidu Netdisk CLI & interactive TUI client (pant)
Moniker: pant
Tags:
  - baidu
  - baidunetdisk
  - cli
  - cloud-storage
  - netdisk
  - tui
ReleaseNotesUrl: https://github.com/{REPO_SLUG}/releases/tag/v{version}
ManifestType: defaultLocale
ManifestVersion: 1.12.0
"""

version_manifest = f"""# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.1.12.0.schema.json

PackageIdentifier: QwerProg.pan-tools
PackageVersion: {version}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.12.0
"""

(ROOT / "pkg/winget").mkdir(parents=True, exist_ok=True)
(ROOT / "pkg/winget/QwerProg.pan-tools.installer.yaml").write_text(installer_manifest, encoding="utf-8")

winget_dir = ROOT / f"pkg/winget-pkgs/manifests/q/QwerProg/pan-tools/{version}"
winget_dir.mkdir(parents=True, exist_ok=True)
(winget_dir / "QwerProg.pan-tools.installer.yaml").write_text(installer_manifest, encoding="utf-8")
(winget_dir / "QwerProg.pan-tools.locale.en-US.yaml").write_text(locale_manifest, encoding="utf-8")
(winget_dir / "QwerProg.pan-tools.yaml").write_text(version_manifest, encoding="utf-8")

# 3. Arch Linux AUR (pan-tools-bin)
aur_dir = ROOT / "pkg/aur-bin"
aur_dir.mkdir(parents=True, exist_ok=True)
(aur_dir / "PKGBUILD").write_text(
    f'''# Maintainer: Chapman <touch65536@gmail.com>
pkgname=pan-tools-bin
pkgver={version}
pkgrel=1
pkgdesc="Modern Baidu Netdisk CLI & interactive TUI client (precompiled binary)"
arch=('x86_64' 'aarch64')
url="https://github.com/{REPO_SLUG}"
license=('Apache-2.0')
provides=('pan-tools')
conflicts=('pan-tools' 'pan-tools-git')
options=('!debug')
source_x86_64=("pant-v${{pkgver}}-linux-amd64.tar.gz::https://github.com/{REPO_SLUG}/releases/download/v${{pkgver}}/pant-v${{pkgver}}-linux-amd64.tar.gz")
source_aarch64=("pant-v${{pkgver}}-linux-arm64.tar.gz::https://github.com/{REPO_SLUG}/releases/download/v${{pkgver}}/pant-v${{pkgver}}-linux-arm64.tar.gz")
sha256sums_x86_64=('{hashes[f'pant-v{version}-linux-amd64.tar.gz']}')
sha256sums_aarch64=('{hashes[f'pant-v{version}-linux-arm64.tar.gz']}')

package() {{
  local _arch="amd64"
  if [[ "$CARCH" == "aarch64" ]]; then
    _arch="arm64"
  fi
  local _dir="${{srcdir}}/pant-v${{pkgver}}-linux-${{_arch}}"
  install -Dm755 "${{_dir}}/pant" "${{pkgdir}}/usr/bin/pant"
  install -Dm644 "${{_dir}}/LICENSE" "${{pkgdir}}/usr/share/licenses/${{pkgname}}/LICENSE"
  install -Dm644 "${{_dir}}/NOTICE" "${{pkgdir}}/usr/share/licenses/${{pkgname}}/NOTICE"
  install -Dm644 "${{_dir}}/README.md" "${{pkgdir}}/usr/share/doc/${{pkgname}}/README.md"
}}
''',
    encoding="utf-8",
)

if shutil.which("makepkg"):
    srcinfo = subprocess.check_output(["makepkg", "--printsrcinfo"], cwd=aur_dir, text=True)
    (aur_dir / ".SRCINFO").write_text(srcinfo, encoding="utf-8")

# 4. Antigravity AI Agent Plugin (plugins/pan-tools/plugin.json)
plugin_manifest = ROOT / "plugins/pan-tools/plugin.json"
if plugin_manifest.exists():
    plugin_data = json.loads(plugin_manifest.read_text(encoding="utf-8"))
    plugin_data["version"] = version
    plugin_manifest.write_text(
        json.dumps(plugin_data, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )

print(f"Updated Homebrew, WinGet, AUR, and Antigravity plugin manifests to {version}")
