# ScreenMirror

A Windows tool that shows your phone's screen on your PC, live. Works with
Android and iPhone/iPad.

## Quick start (Android)

1. **[Download screenmirror.exe](https://github.com/elshaddai01/screenmirror/releases/latest/download/screenmirror.exe)**
   and double-click it. Nothing to install — it works immediately.
2. **Plug your phone into the PC with a USB cable.**
3. **Tap "Allow" on your phone** when it asks about USB debugging.

Your phone's screen now shows up in its own window. That's the whole
process — no typing, no apps to install on the phone.

No cable? No iPhone? See [How to mirror any phone](#how-to-mirror-any-phone)
below for Wi-Fi and iOS instructions.

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
- Cleans up automatically: when a device disconnects, its window closes and
  its child process is killed — no orphaned scrcpy/uxplay processes.
- Detects missing dependencies at startup and prints exact install commands
  instead of failing silently.

## Prerequisites

**Android: none.** `adb.exe` and `scrcpy.exe` are embedded in
`screenmirror.exe` itself (see `internal/bundle`) and are extracted
automatically to a per-user cache folder (`%LocalAppData%\ScreenMirror\tools`)
the first time you run it — nothing to install, nothing on PATH, works
completely offline.

**iOS is the one thing that still needs a one-time manual step.** UxPlay
depends on a GStreamer runtime (100+ MB of DLLs) that's too large to bundle
sight-unseen, has no winget/Chocolatey package on Windows, and must be built
from source via MSYS2 — see the message ScreenMirror itself prints on
startup if `uxplay.exe` isn't found, or
[UxPlay's README](https://github.com/FDH2/UxPlay) directly. Skip this
entirely if you only need Android mirroring.

## Building

Only needed if you want to build it yourself instead of using a released
`screenmirror.exe`. Requires Go 1.24+.

```powershell
go build -o screenmirror.exe .
```

## Running

Double-click `screenmirror.exe`, or from a terminal:

```powershell
.\screenmirror.exe
```

No arguments required. Flags, all optional:

- `-v` — verbose/debug logging
- `-ios-pool N` — number of simultaneous AirPlay receiver slots (default 2)
- `-name NAME` — the AirPlay receiver name advertised on the network (default
  `ScreenMirror-<hostname>`)

## How to mirror any phone

Every method below ends the same way: a titled window pops up on its own,
no command typed after launch. What differs is the one-time trust step each
platform/connection type requires.

### Android — wired (USB)

1. Enable Developer Options on the phone if you haven't already: **Settings
   → About phone → tap "Build number" 7 times**, then **Settings →
   Developer options → USB debugging → on**.
2. Plug the phone into this PC with a USB cable.
3. The phone shows **"Allow USB debugging from this computer?"** — tap
   **Allow** (optionally tick "Always allow from this computer" so it's
   truly one-time). ScreenMirror is watching continuously, so the mirror
   window appears within a second or two of you tapping Allow — nothing to
   run.

**If nothing happens:**
- **No popup appeared on the phone at all** — the cable is probably
  charge-only. Try a different cable, or a different USB port (rear/
  motherboard ports are more reliable than front-panel/hub ports).
- **Popup appeared, you tapped Allow, still nothing** — run
  `adb devices -l` in a separate terminal. If it shows `unauthorized`, the
  trust prompt didn't register; unplug, re-plug, and tap Allow again. If it
  shows nothing at all, Windows may not have a driver for the phone —
  install the manufacturer's USB driver (Google's [universal ADB
  driver](https://developer.android.com/studio/run/win-usb) covers most
  Pixel/AOSP-based phones; Samsung, Xiaomi, etc. need their own OEM driver).
- **Window opens then instantly closes** — run with `-v` and check the log
  line `mirror window closed: ... (exit status N)`. This is scrcpy itself
  reporting an error, almost always because **two things are racing for the
  same device**: two ScreenMirror instances running at once (check Task
  Manager for more than one `screenmirror.exe`, or `scrcpy.exe` launched by
  something else — Android Studio, Vysor, another scrcpy GUI) are the most
  common cause. Close the duplicate and reconnect the cable.

### Android — wireless (Wi-Fi)

1. Plug the phone in via USB once and get it authorized as in the wired
   section above.
2. In a terminal: `adb tcpip 5555`
3. Unplug the USB cable.
4. Find the phone's IP address (**Settings → About phone → Status → IP
   address**, or **Settings → Wi-Fi → tap the connected network**).
5. In a terminal: `adb connect <phone-ip>:5555`

ScreenMirror is watching continuously and opens the mirror window the
instant `adb connect` succeeds — no restart needed.

**If it doesn't work:**
- **`adb connect` says "already connected" but no window appears** — this
  is almost always the *two-instances* problem again: check Task Manager
  for more than one `screenmirror.exe`. Only one should ever run — kill any
  extra copy and re-run `adb disconnect <phone-ip>:5555` followed by
  `adb connect <phone-ip>:5555` to force a fresh connect event.
- **`adb connect` says "failed to connect" / times out** — the phone's IP
  changed (common after it reconnects to Wi-Fi or reboots, since `adb
  tcpip` mode doesn't survive a reboot or USB replug), or the phone is on a
  different subnet/isolated network than the PC. Re-check the IP and that
  both devices show the same Wi-Fi network.
- **Window opens then closes immediately** — same guidance as the wired
  section: check for a duplicate scrcpy/ScreenMirror process racing for the
  same device.

### iOS — AirPlay (iPhone/iPad)

Requires `uxplay.exe` — see Prerequisites above if ScreenMirror printed a
setup message for it. There is no wired iOS option on Windows at all; this
is the only path, by Apple's own design.

1. Make sure the iPhone/iPad is on the **same Wi-Fi network** as this PC.
2. Open **Control Center** (swipe down from the top-right corner, or up
   from the bottom on older iPhones) and tap **Screen Mirroring**.
3. Pick this PC's name from the list — by default `ScreenMirror-<hostname>`
   (or whatever you passed to `-name`).
4. The mirror window appears within a second or two. **This selection step
   has to be repeated every session** — Apple doesn't let any AirPlay
   receiver skip it, on any platform.

**If nothing happens:**
- **PC's name doesn't show up in Screen Mirroring at all** — this is an
  mDNS/Bonjour discovery failure: confirm both devices are truly on the
  same Wi-Fi network (not guest/isolated), and allow
  `uxplay.exe`/`screenmirror.exe` through Windows Firewall (Private *and*
  Public) if prompted.
- **Name shows up, tapping it does nothing / mirrors then immediately
  stops** — run with `-v` and check the `[airplay:...]` log lines for
  errors. This is usually a missing GStreamer plugin (rebuild UxPlay
  exactly per its own README's package list) or another app already
  holding the AirPlay port range.
- **Window shows up untitled or generically labeled
  `iOS Device (ScreenMirror-...)` instead of the device's real name** —
  harmless; UxPlay's log format for the device name varies by build and
  ScreenMirror's name-scraping is best-effort only. Mirroring itself is
  unaffected. Run with `-v` and check the raw `[airplay:...]` lines if you
  want to tune the matching patterns in `internal/ios/uxplay.go`.
- **Only one iPhone can mirror even though `-ios-pool` is higher than 1** —
  each pool slot advertises a *separate* name (`ScreenMirror-1`,
  `ScreenMirror-2`, ...); make sure each phone picked a different one from
  Screen Mirroring, not the same slot twice.

### General troubleshooting

- **Only ever run one `screenmirror.exe` at a time.** Two instances both
  watch adb and both race to launch scrcpy for the same device, which
  reliably causes windows to flash open and immediately close (visible as
  `exit status 1`/`2` in the log). Check Task Manager if this happens.
- **Run with `-v`** any time something doesn't behave as expected — every
  connect/disconnect/error is logged with enough detail to tell you exactly
  which stage failed (adb tracking, scrcpy launch, AirPlay client
  detection).
- **adb server conflicts:** if Android Studio, Vysor, or another scrcpy GUI
  is also running, they all share the *same* background `adb` server
  process — that's normally fine (ScreenMirror just observes its device
  list), but if devices behave strangely, close the other tool and run
  `adb kill-server` once, then restart ScreenMirror so it starts a clean
  server itself.
- **Firewall:** the first run may trigger a Windows Firewall prompt for
  `screenmirror.exe` (needed for AirPlay discovery) — allow it on both
  Private and Public networks, or iOS discovery won't work even though
  Android mirroring still will.

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

## Project layout

```
main.go                          orchestrator entrypoint, dependency checks
internal/bundle/                 embedded adb.exe/scrcpy.exe, extracted on first run
internal/adbproto/               raw ADB host-protocol client (track-devices)
internal/android/                device→scrcpy lifecycle management
internal/ios/                    UxPlay receiver pool + window detection
internal/tile/                   window position/size grid allocator
internal/winapi/                 Win32 window title/position/enumeration
internal/procutil/               subprocess console-hiding helper
internal/deps/                   dependency detection + setup messages
internal/logging/                leveled console logger
internal/orchestrator/           wires everything together
```
