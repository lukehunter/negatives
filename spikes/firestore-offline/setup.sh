#!/bin/sh
# Generates FirestoreOfflineSpike.xcodeproj. Reads the Google client ID from the
# GoogleService-Info.plist outside the repo, so nothing from it is committed.
set -eu
cd "$(dirname "$0")"

plist="$HOME/Library/Application Support/negatives/GoogleService-Info.plist"
if [ ! -f "$plist" ]; then
  echo "missing $plist" >&2
  exit 1
fi
reversed=$(/usr/libexec/PlistBuddy -c "Print REVERSED_CLIENT_ID" "$plist")

printf '%s\nGOOGLE_REVERSED_CLIENT_ID = %s\n' \
  '// Written by setup.sh from GoogleService-Info.plist. Not committed.' "$reversed" > Local.xcconfig

if ! command -v xcodegen >/dev/null; then
  echo "XcodeGen is missing: brew install xcodegen" >&2
  exit 1
fi
xcodegen generate
