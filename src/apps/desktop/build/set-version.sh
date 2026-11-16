#!/usr/bin/env bash
# Stamps a release version into the desktop app before building.
#
#   build/set-version.sh 1.0.1
#
# It writes version.txt (read by the app at runtime) and the version fields of
# the packaging files (Windows resources and installer, macOS Info.plist,
# Linux packages). Run from src/apps/desktop. Portable: bash and perl only,
# as found on the Linux, macOS and Windows (Git Bash) GitHub runners.
set -euo pipefail

version="${1:-}"
if ! [[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "error: version must be MAJOR.MINOR.PATCH, got '$version'" >&2
  exit 1
fi

export VERSION="$version"
printf '%s\n' "$version" > version.txt
perl -pi -e 's/^(  version: ")[^"]*(")/${1}$ENV{VERSION}${2}/' build/config.yml
perl -pi -e 's/^(version: ")[^"]*(")/${1}$ENV{VERSION}${2}/' build/linux/nfpm/nfpm.yaml
perl -pi -e 's/("(?:file_version|product_version|FileVersion|ProductVersion)": ")[^"]*(")/${1}$ENV{VERSION}${2}/g' build/windows/info.json
perl -pi -e 's/(!define INFO_PRODUCTVERSION ")[^"]*(")/${1}$ENV{VERSION}${2}/' build/windows/nsis/wails_tools.nsh
# Info.plist: the <string> following CFBundleShortVersionString and CFBundleVersion.
perl -0pi -e 's{(<key>CFBundle(?:Short)?Version(?:String)?</key>\s*<string>)[^<]*(</string>)}{$1$ENV{VERSION}$2}g' build/darwin/Info.plist

echo "version $version stamped"
