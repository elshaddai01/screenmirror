// Package deps detects whether ScreenMirror's external dependencies
// (adb.exe, scrcpy.exe, uxplay.exe) are present on PATH, and builds a clear
// one-time setup message if not.
package deps

import (
	"os/exec"
	"strings"
)

// Check looks up each dependency on PATH. adbPath/scrcpyPath/uxplayPath are
// "" when not found. message is non-empty (and should be printed) whenever
// anything is missing.
func Check() (adbPath, scrcpyPath, uxplayPath, message string) {
	adbPath = lookup("adb")
	scrcpyPath = lookup("scrcpy")
	uxplayPath = lookup("uxplay")

	var missing []string
	if adbPath == "" {
		missing = append(missing, adbSetup)
	}
	if scrcpyPath == "" {
		missing = append(missing, scrcpySetup)
	}
	if uxplayPath == "" {
		missing = append(missing, uxplaySetup)
	}
	if len(missing) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("\n=== ScreenMirror: one-time setup required ===\n\n")
	for i, m := range missing {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m)
		b.WriteString("\n")
	}
	b.WriteString("\nAfter installing, restart your terminal (so PATH updates take effect) and run ScreenMirror again.\n")
	if adbPath != "" && scrcpyPath != "" && uxplayPath == "" {
		b.WriteString("Android mirroring will work in the meantime; only iOS AirPlay mirroring is unavailable until UxPlay is installed.\n")
	}
	b.WriteString("================================================\n")
	return adbPath, scrcpyPath, uxplayPath, b.String()
}

func lookup(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

const adbSetup = `- ADB (adb.exe) not found on PATH. Needed for Android device detection.
    winget:      winget install --id Google.PlatformTools -e
    Chocolatey:  choco install adb
    Manual:      download "SDK Platform-Tools for Windows" from
                 https://developer.android.com/tools/releases/platform-tools
                 and add the extracted folder to your PATH.`

const scrcpySetup = `- scrcpy (scrcpy.exe) not found on PATH. Needed to actually render the Android mirror.
    winget:      winget install --id Genymobile.scrcpy -e
    Chocolatey:  choco install scrcpy
    (scrcpy bundles its own copy of adb, but ScreenMirror looks for both
    tools on PATH independently, so make sure both resolve.)`

const uxplaySetup = `- UxPlay (uxplay.exe) not found on PATH. Needed only for iOS AirPlay mirroring.
    UxPlay has no winget/Chocolatey package on Windows; build it yourself:
      1. Install MSYS2 (https://www.msys2.org), then in an "MSYS2 UCRT64" shell:
           pacman -S mingw-w64-ucrt-x86_64-toolchain mingw-w64-ucrt-x86_64-cmake \
                     mingw-w64-ucrt-x86_64-ninja mingw-w64-ucrt-x86_64-gstreamer \
                     mingw-w64-ucrt-x86_64-gst-plugins-good mingw-w64-ucrt-x86_64-gst-plugins-bad \
                     mingw-w64-ucrt-x86_64-gst-libav mingw-w64-ucrt-x86_64-libplist \
                     mingw-w64-ucrt-x86_64-openssl git
           git clone https://github.com/FDH2/UxPlay.git && cd UxPlay
           cmake -G Ninja -B build && cmake --build build
      2. Put build/uxplay.exe on your PATH, alongside (or able to find) the
         MSYS2 UCRT64 bin/ GStreamer DLLs it depends on.
    See https://github.com/FDH2/UxPlay for current build instructions.`
