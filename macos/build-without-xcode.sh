#!/bin/sh
# macos/build-without-xcode.sh — build Driver.app with the Command Line Tools
# alone: no Xcode, no xcodegen, no Apple team. The Makefile's `mac` and
# `install-mac` run this when DRIVER_MAC_BUILDER resolves to `clt` (its
# default wherever the active developer dir is the Command Line Tools rather
# than Xcode — see the Makefile preamble); the xcodegen + xcodebuild recipe
# beside it is the reference this script imitates, and project.yml stays the
# one source of the settings both read.
#
# What xcodebuild would do, reproduced by hand in the same order:
#   1. compile the Swift files under macos/Driver into one executable for
#      this machine's architecture, with the settings project.yml pins — the
#      deployment target, the Swift language mode (major only: swiftc takes
#      4, 4.2, 5 or 6, and the project's "5.10" is a toolchain version rather
#      than a mode), the module name;
#   2. write Contents/Info.plist from macos/Generated-Info.plist — the tracked
#      xcodegen rendering of project.yml's `info:` block — with its
#      $(BUILD_SETTING) placeholders substituted from the same project.yml
#      values, plus the keys xcodebuild adds on its own (LSMinimumSystemVersion,
#      NSPrincipalClass, NSHighResolutionCapable);
#   3. the icon: a classic .icns rendered from AppIcon-1024.png beside this
#      script (make-icon.swift draws it) — the same sips + iconutil recipe
#      project.yml runs as a post-build phase under Xcode; no asset catalog,
#      since actool ships only with Xcode;
#   4. ad-hoc code-sign the bundle under project.yml's bundle identifier:
#      there is no team here, so the app runs only where an ad-hoc signature
#      may — which on a Mac whose endpoint security allows local builds from
#      the source tree alone is why the Makefile's DRIVER_APP_DIR knob exists.
#
# Usage: build-without-xcode.sh <out-dir>      → <out-dir>/Driver.app
# Needs: xcrun + swiftc (`xcode-select --install`), sips, iconutil, plutil
# and codesign — all of them macOS and the Command Line Tools.
set -eu

out=${1:?usage: build-without-xcode.sh <out-dir>}
here=$(cd "$(dirname "$0")" && pwd)
spec="$here/project.yml"

die() {
	echo "build-without-xcode.sh: $*" >&2
	exit 1
}

# setting KEY prints the value of the first `KEY: value` line of project.yml,
# a trailing `# comment` and wrapping quotes dropped. A key with no inline
# value (a map, like `settings:`) does not match, which is also how a missing
# key reads: empty, and `require` below turns that into a named refusal.
setting() {
	sed -n "s/^[[:space:]]*$1:[[:space:]]*\(..*\)/\1/p" "$spec" | head -n 1 |
		sed -e 's/[[:space:]]*#.*$//' -e 's/^"\(.*\)"$/\1/' -e 's/[[:space:]]*$//'
}
require() {
	[ -n "$2" ] || die "$1 has no value in $spec"
}

bundle_id=$(setting PRODUCT_BUNDLE_IDENTIFIER)
product=$(setting PRODUCT_NAME)
marketing=$(setting MARKETING_VERSION)
build_no=$(setting CURRENT_PROJECT_VERSION)
swift_version=$(setting SWIFT_VERSION)
min_os=$(setting deploymentTarget)
require PRODUCT_BUNDLE_IDENTIFIER "$bundle_id"
require PRODUCT_NAME "$product"
require MARKETING_VERSION "$marketing"
require CURRENT_PROJECT_VERSION "$build_no"
require SWIFT_VERSION "$swift_version"
require deploymentTarget "$min_os"
swift_mode=${swift_version%%.*}
arch=$(uname -m)

app="$out/$product.app"
work=$(mktemp -d /tmp/driver-mac-clt.XXXXXX)
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

# 1. the executable
echo "swiftc: $product for $arch, macOS $min_os+, Swift $swift_mode mode"
xcrun swiftc -O -swift-version "$swift_mode" -parse-as-library -module-name "$product" \
	-target "$arch-apple-macos$min_os" \
	-o "$app/Contents/MacOS/$product" "$here"/Driver/*.swift

# 2. Info.plist — the generated plist's $(SETTING) placeholders substituted,
#    then the keys xcodebuild adds itself. A placeholder this list does not
#    know stays in the file and fails the build rather than shipping as the
#    literal text "$(…)", which Launch Services would read as the value.
plist="$app/Contents/Info.plist"
cp "$here/Generated-Info.plist" "$plist"
for pair in "DEVELOPMENT_LANGUAGE=en" "EXECUTABLE_NAME=$product" \
	"PRODUCT_BUNDLE_IDENTIFIER=$bundle_id" "PRODUCT_NAME=$product" \
	"MARKETING_VERSION=$marketing" "CURRENT_PROJECT_VERSION=$build_no"; do
	key=${pair%%=*}
	val=${pair#*=}
	sed -e "s|\\\$($key)|$val|g" "$plist" >"$work/Info.plist"
	mv "$work/Info.plist" "$plist"
done
if grep -q '\$(' "$plist"; then
	grep '\$(' "$plist" >&2
	die "Info.plist carries a build setting this script does not substitute (above)"
fi
plutil -replace LSMinimumSystemVersion -string "$min_os" "$plist"
plutil -replace NSPrincipalClass -string NSApplication "$plist"
plutil -replace NSHighResolutionCapable -bool true "$plist"
plutil -lint "$plist" >/dev/null
printf 'APPL????' >"$app/Contents/PkgInfo"

# 3. the icon: every size iconutil wants, scaled from the one 1024px source
#    (the plist's CFBundleIconFile already names AppIcon)
iconset="$work/AppIcon.iconset"
mkdir -p "$iconset"
for px in 16 32 128 256 512; do
	sips -z "$px" "$px" "$here/AppIcon-1024.png" --out "$iconset/icon_${px}x${px}.png" >/dev/null
	sips -z $((px * 2)) $((px * 2)) "$here/AppIcon-1024.png" --out "$iconset/icon_${px}x${px}@2x.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$app/Contents/Resources/AppIcon.icns"

# 4. the signature
codesign --force --sign - --identifier "$bundle_id" "$app"
codesign --verify --strict "$app"
echo "built $app"
