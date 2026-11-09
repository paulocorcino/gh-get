#!/usr/bin/env python3
"""Download the pinned lychee release for this platform, check its sha256, extract it.

Usage: fetch-lychee.py <dir>   prints the path of the lychee binary.
The pin (version and checksums) lives here; a bump is its own commit, re-measured.
"""

import hashlib
import io
import os
import platform
import sys
import tarfile
import urllib.request
import zipfile

VERSION = "v0.24.2"
# asset -> sha256 (from the .sha256 files of the release, checked against the downloaded bytes)
ASSETS = {
    "x86_64-pc-windows-msvc.zip": "32975d1493ee1a975d6bb41e4fb56fe419cb442ded628bb772ba2e614acfacad",
    "x86_64-unknown-linux-gnu.tar.gz": "1f4e0ef7f6554a6ed33dd7ac144fb2e1bbed98598e7af973042fc5cd43951c9a",
    "x86_64-apple-darwin.tar.gz": "887503a9cff667d322b8d0892b40bf49976eb9507af8483220a3706cdad55978",
    "aarch64-apple-darwin.tar.gz": "c9d3740ea2d891854d37116c9fba840f37b6e7c89d330e7db84ac333631c4977",
}


def asset():
    system, machine = platform.system(), platform.machine().lower()
    arm = machine in ("arm64", "aarch64")
    if system == "Windows":
        return "x86_64-pc-windows-msvc.zip"
    if system == "Darwin":
        return "aarch64-apple-darwin.tar.gz" if arm else "x86_64-apple-darwin.tar.gz"
    return "x86_64-unknown-linux-gnu.tar.gz"


def main():
    dest = sys.argv[1]
    name = asset()
    url = f"https://github.com/lycheeverse/lychee/releases/download/lychee-{VERSION}/lychee-{name}"
    with urllib.request.urlopen(url, timeout=60) as r:  # noqa: S310 - fixed https URL
        data = r.read()
    got = hashlib.sha256(data).hexdigest()
    if got != ASSETS[name]:
        sys.exit(f"lychee {name}: sha256 {got} does not match the pin {ASSETS[name]}")
    if name.endswith(".zip"):
        zipfile.ZipFile(io.BytesIO(data)).extractall(dest)
    else:
        with tarfile.open(fileobj=io.BytesIO(data)) as t:
            t.extractall(dest, filter="data")
    for root, _, files in os.walk(dest):
        for f in files:
            if f in ("lychee", "lychee.exe"):
                print(os.path.join(root, f))
                return
    sys.exit("lychee binary not found in the archive")


if __name__ == "__main__":
    main()
