// Package bundle embeds a complete, self-contained copy of adb.exe and
// scrcpy.exe (plus every DLL scrcpy needs) directly into the ScreenMirror
// binary, so Android mirroring works the instant the .exe is double-clicked
// -- no winget, no separate download, no PATH setup.
//
// iOS/UxPlay is deliberately NOT bundled here: it depends on a GStreamer
// runtime that's an order of magnitude larger (100+MB of DLLs) than
// everything else combined, and has no Windows-portable redistribution
// that's safe to embed sight-unseen. It keeps its existing PATH-based
// detection and one-time manual setup instructions.
//
// adb and scrcpy are both distributed under the Apache License 2.0; see
// THIRD_PARTY_LICENSES/ in the repository root.
package bundle

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets
var assets embed.FS

// bundleVersion is bumped whenever the embedded assets change, so Extract
// knows to overwrite a stale previously-extracted copy rather than trusting
// whatever's already on disk.
const bundleVersion = "1"

// Extract writes the embedded adb/scrcpy files to a per-user cache
// directory (skipping the work if an up-to-date copy is already there) and
// returns the absolute paths to adb.exe and scrcpy.exe.
func Extract() (adbPath, scrcpyPath string, err error) {
	dir, err := targetDir()
	if err != nil {
		return "", "", err
	}

	versionFile := filepath.Join(dir, ".version")
	if v, err := os.ReadFile(versionFile); err == nil && string(v) == bundleVersion {
		if adbPath, scrcpyPath, ok := existing(dir); ok {
			return adbPath, scrcpyPath, nil
		}
	}

	if err := extractAll(dir); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(versionFile, []byte(bundleVersion), 0o644); err != nil {
		return "", "", err
	}

	adbPath, scrcpyPath, ok := existing(dir)
	if !ok {
		return "", "", fmt.Errorf("extraction to %s appeared to succeed but adb.exe/scrcpy.exe are missing", dir)
	}
	return adbPath, scrcpyPath, nil
}

func targetDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating a cache directory: %w", err)
	}
	dir := filepath.Join(cacheDir, "ScreenMirror", "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	return dir, nil
}

func existing(dir string) (adbPath, scrcpyPath string, ok bool) {
	adbPath = filepath.Join(dir, "adb.exe")
	scrcpyPath = filepath.Join(dir, "scrcpy.exe")
	if fileExists(adbPath) && fileExists(scrcpyPath) {
		return adbPath, scrcpyPath, true
	}
	return "", "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func extractAll(dir string) error {
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		return fmt.Errorf("reading embedded asset list: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src, err := assets.Open("assets/" + entry.Name())
		if err != nil {
			return fmt.Errorf("opening embedded %s: %w", entry.Name(), err)
		}
		if err := writeFile(filepath.Join(dir, entry.Name()), src); err != nil {
			src.Close()
			return err
		}
		src.Close()
	}
	return nil
}

func writeFile(destPath string, src io.Reader) error {
	tmp := destPath + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("creating %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		return err
	}
	// A file that's currently running (e.g. a leftover adb.exe server
	// process) can't be overwritten directly on Windows; renaming over it
	// works even while it's in use.
	if err := os.Rename(tmp, destPath); err != nil {
		return fmt.Errorf("installing %s: %w", destPath, err)
	}
	return nil
}
