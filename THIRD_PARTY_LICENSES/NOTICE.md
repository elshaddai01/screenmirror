# Third-party components

`screenmirror.exe` embeds and redistributes pre-built binaries from two
upstream projects, both under the Apache License 2.0 (full text in this
directory):

- **adb.exe, AdbWinApi.dll, AdbWinUsbApi.dll, scrcpy-server** — from
  [Android platform-tools](https://developer.android.com/tools/releases/platform-tools),
  © The Android Open Source Project.
- **scrcpy.exe** and its runtime DLLs (SDL3.dll, avcodec/avformat/avutil,
  swresample, libusb-1.0) — from
  [Genymobile/scrcpy](https://github.com/Genymobile/scrcpy) (`scrcpy_LICENSE.txt`),
  which itself bundles FFmpeg (LGPL/GPL depending on build; the official
  Windows release build used here is LGPL), SDL (zlib license), and libusb
  (LGPL-2.1).

None of these binaries are modified — they're redistributed exactly as
published in their official Windows release archives.

The ADB wireless (QR) pairing protocol implementation in
`internal/pairing/` is original Go source code, not a redistributed binary,
written from AOSP's `packages/modules/adb/pairing_auth` and
`pairing_connection` (Apache License 2.0) and cross-checked against
BoringSSL's `crypto/curve25519/spake25519.cc` (Apache License 2.0) for
protocol/algorithm compatibility — no code was copied from either.
