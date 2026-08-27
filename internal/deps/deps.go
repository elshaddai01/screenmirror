// Package deps detects whether ScreenMirror's external dependencies are
// available and builds a clear one-time setup message for whatever isn't.
//
// adb.exe and scrcpy.exe are embedded directly in the binary (see
// internal/bundle) and never need installing. uxplay.exe is the one
// remaining dependency still looked up on PATH -- see the doc comment on
// internal/bundle for why it isn't embedded too.
package deps

import "os/exec"

// Check looks up uxplay on PATH. uxplayPath is "" when not found. message is
// non-empty (and should be printed) when it's missing.
func Check() (uxplayPath, message string) {
	uxplayPath = lookup("uxplay")
	if uxplayPath != "" {
		return uxplayPath, ""
	}
	return "", "\n=== ScreenMirror: optional setup ===\n\n" + uxplaySetup +
		"\n\nAndroid mirroring works right now with no setup. This step is only needed\n" +
		"for iOS AirPlay mirroring; restart your terminal after installing and run\n" +
		"ScreenMirror again.\n================================================\n"
}

func lookup(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

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
