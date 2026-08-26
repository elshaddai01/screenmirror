# ScreenMirror

A zero-configuration Windows command-line tool that mirrors the live screen
of every connected phone the moment it's plugged in or paired — Android over
ADB/scrcpy, iOS over AirPlay/UxPlay. No commands to type after launch, no
per-device setup beyond Android's own one-time USB trust prompt and iOS's
own one-time-per-session AirPlay device selection.

## What it does

- Watches ADB's device list continuously (by speaking the `adb track-devices`
  host protocol directly over the local adb server socket, not by shelling
  out and scraping CLI output). The instant an Android device shows up
  authorized, USB or Wi-Fi, it launches a titled, tiled
  [scrcpy](https://github.com/Genymobile/scrcpy) window for it.
- Runs a pool of [UxPlay](https://github.com/FDH2/UxPlay) AirPlay receivers
  so multiple iPhones/iPads can mirror to this PC at once, each getting its
  own titled, tiled window (title/position are applied via Win32 window
  APIs, since UxPlay itself doesn't expose that).
- Advertises a QR code (Settings → Developer options → Wireless debugging →
  **Pair device with QR code**) that pairs a phone over Wi-Fi with zero
  typing, using a from-scratch Go implementation of Android's ADB TLS
  pairing protocol (SPAKE2 over Curve25519 + AES-128-GCM, ported from AOSP's
  `pairing_auth`/`pairing_connection` C++ source and cross-checked against
  BoringSSL's `spake25519.cc`). Once paired, the phone trusts this PC's own
  adb key, so adb's built-in mDNS auto-connect picks it up with no further
  prompts.
- Cleans up automatically: when a device disconnects, its window closes and
  its child process is killed — no orphaned scrcpy/uxplay processes.
- Detects missing dependencies at startup and prints exact install commands
  instead of failing silently.

## Prerequisites

Install once:

```powershell
winget install --id Google.PlatformTools -e   # adb
winget install --id Genymobile.scrcpy -e      # scrcpy
```

UxPlay (only needed for iOS AirPlay mirroring) has no winget/Chocolatey
package on Windows and must be built from source via MSYS2 — see the message
ScreenMirror itself prints on startup if `uxplay.exe` isn't found, or
[UxPlay's README](https://github.com/FDH2/UxPlay) directly.

**Restart your terminal after installing** so PATH updates take effect.

## Building

Requires Go 1.24+.

```powershell
go build -o screenmirror.exe .
```

## Running

```powershell
.\screenmirror.exe
```

No arguments required. Flags, all optional:

- `-v` — verbose/debug logging
- `-ios-pool N` — number of simultaneous AirPlay receiver slots (default 2)
- `-name NAME` — the AirPlay/pairing name advertised on the network (default
  `ScreenMirror-<hostname>`)

## One-time device setup

**Android, wired:** plug in the phone. It'll show "Allow USB debugging from
this computer?" — tap Allow once. The mirror window appears within a couple
seconds.

**Android, wireless:** either

- Pair once with **Settings → Developer options → Wireless debugging →
  Pair device with QR code**, scanning the QR code ScreenMirror prints on
  startup — no typing needed, and future connections need no further
  action since adb's own mDNS auto-connect takes over; or
- the classic bootstrap method: plug in via USB once, run `adb tcpip 5555`,
  unplug, then `adb connect <phone-ip>:5555`.

**iOS:** on the same Wi-Fi network, open Control Center → Screen Mirroring
and pick this PC's advertised name. This has to be done manually every
session — it's an Apple platform restriction on all AirPlay receivers, not
something any app on Windows can bypass.

## Known limitations

- Windows has no wired/USB iOS mirroring path at all (Apple doesn't allow
  it) — AirPlay over Wi-Fi is the only option in scope here.
- iOS AirPlay device selection is manual every session; this tool can make
  Android fully hands-off but not iOS's own OS-level picker.
- Simultaneous multi-iPhone mirroring depends on running `-ios-pool` separate
  UxPlay receiver instances (each with a distinct advertised name the user
  picks between), since a single UxPlay process generally only holds one
  active AirPlay client at a time.
- UxPlay's exact log output (used only to label AirPlay windows with a
  device name) varies by build/version; if a name can't be scraped, the
  window is labeled generically. The mirroring itself is unaffected — window
  connect/disconnect is detected via the actual OS window lifecycle, not log
  parsing.
- The QR wireless-pairing implementation reimplements Android's pairing
  crypto from protocol documentation and AOSP source rather than calling a
  first-party library (none is exposed by `adb.exe`), and is not
  constant-time — an acceptable trade-off for pairing your own phone on your
  own LAN, not a general-purpose security library.

## Project layout

```
main.go                          orchestrator entrypoint, dependency checks
internal/adbproto/               raw ADB host-protocol client (track-devices)
internal/android/                device→scrcpy lifecycle management
internal/ios/                    UxPlay receiver pool + window detection
internal/pairing/                ADB wireless (QR) pairing protocol + mDNS
internal/tile/                   window position/size grid allocator
internal/winapi/                 Win32 window title/position/enumeration
internal/procutil/               subprocess console-hiding helper
internal/deps/                   dependency detection + setup messages
internal/logging/                leveled console logger
internal/orchestrator/           wires everything together
```
