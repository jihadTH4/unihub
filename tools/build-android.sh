#!/usr/bin/env bash
# Builds the Android app: the Go engine for the phone's processors, then the APK.
# usage: tools/build-android.sh <version> <release|debug> [owner/repo]
# For a signed release set UNIHUB_KEYSTORE, UNIHUB_KEYSTORE_PASSWORD (and optionally UNIHUB_KEY_ALIAS).
set -euo pipefail
VERSION="$1"
VARIANT="${2:-release}"
REPO="${3:-jihadTH4/unihub}"
cd "$(dirname "$0")/.."

JNI=android/app/src/main/jniLibs
rm -rf "$JNI"
mkdir -p "$JNI/arm64-v8a"
LDF="-s -w -X main.Version=$VERSION -X main.UpdateRepo=$REPO"

echo "engine: arm64"
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags "$LDF" -o "$JNI/arm64-v8a/libunihub.so" .

# older 32-bit phones: Go needs the NDK's C linker for android/arm
NDK="${ANDROID_NDK_LATEST_HOME:-${ANDROID_NDK_HOME:-${ANDROID_NDK_ROOT:-}}}"
CC_ARM="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/armv7a-linux-androideabi26-clang"
if [ -n "$NDK" ] && [ -x "$CC_ARM" ]; then
  echo "engine: arm (32-bit)"
  mkdir -p "$JNI/armeabi-v7a"
  if ! CGO_ENABLED=1 CC="$CC_ARM" GOOS=android GOARCH=arm GOARM=7 \
      go build -trimpath -ldflags "$LDF" -o "$JNI/armeabi-v7a/libunihub.so" .; then
    echo "::warning::32-bit engine failed to build; the APK will support 64-bit phones only"
    rm -rf "$JNI/armeabi-v7a"
  fi
else
  echo "::warning::Android NDK not found; the APK will support 64-bit phones only"
fi

mkdir -p dist
cd android
if [ "$VARIANT" = "debug" ]; then
  gradle --no-daemon --stacktrace assembleDebug
  cp app/build/outputs/apk/debug/app-debug.apk ../dist/UniHub-debug.apk
else
  gradle --no-daemon --stacktrace assembleRelease
  cp app/build/outputs/apk/release/app-release.apk ../dist/UniHub.apk
fi
