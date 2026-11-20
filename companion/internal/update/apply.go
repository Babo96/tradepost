package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// MaxAssetSize caps the downloaded release archive.
	MaxAssetSize = 200 << 20
	// maxExtractedSize caps the total uncompressed archive contents.
	maxExtractedSize = 500 << 20
	downloadTimeout  = 10 * time.Minute
)

// ApplyOptions controls Apply. Tests use CurrentExe and TempDir to avoid
// touching the running binary.
type ApplyOptions struct {
	// CurrentExe overrides the executable to replace. Defaults to
	// os.Executable().
	CurrentExe string
	// TempDir is the parent directory for the download/extract workspace.
	TempDir string
	// Client overrides the HTTP client used for the download.
	Client *http.Client
	// Timeout bounds the whole apply operation. Defaults to 10 minutes.
	Timeout time.Duration
}

// ApplyResult describes a completed install. SourceDir is inside the returned
// temporary workspace; call Cleanup after installing the addon from it.
type ApplyResult struct {
	SourceDir       string
	BinaryPath      string
	Version         string
	RestartRequired bool

	tempDir string
}

// Cleanup removes the temporary download and extraction directory.
func (r *ApplyResult) Cleanup() error {
	if r == nil || r.tempDir == "" {
		return nil
	}
	dir := r.tempDir
	r.tempDir = ""
	return os.RemoveAll(dir)
}

// Apply downloads the release asset, verifies its digest when present,
// extracts it, and replaces the current executable. The running process keeps
// using the old code; RestartRequired reports whether a restart is needed.
func Apply(ctx context.Context, info Info, opts ApplyOptions) (*ApplyResult, error) {
	if strings.TrimSpace(info.AssetURL) == "" {
		return nil, errors.New("no release asset available for this platform")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = downloadTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tempDir, err := os.MkdirTemp(opts.TempDir, "tradepost-update-*")
	if err != nil {
		return nil, fmt.Errorf("create temp directory: %w", err)
	}
	result := &ApplyResult{Version: NormalizeVersion(info.Latest), tempDir: tempDir}
	fail := func(err error) (*ApplyResult, error) {
		_ = result.Cleanup()
		return nil, err
	}

	archivePath := filepath.Join(tempDir, "download"+archiveExtension(info.AssetName))
	if err := downloadAsset(ctx, info, archivePath, opts.Client); err != nil {
		return fail(err)
	}

	extractDir := filepath.Join(tempDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fail(fmt.Errorf("prepare extract directory: %w", err))
	}
	if err := extractArchive(archivePath, extractDir); err != nil {
		return fail(fmt.Errorf("extract %s: %w", info.AssetName, err))
	}

	sourceDir, err := findSourceRoot(extractDir)
	if err != nil {
		return fail(err)
	}
	newBinary, err := findBinary(sourceDir)
	if err != nil {
		return fail(err)
	}

	currentExe := opts.CurrentExe
	if currentExe == "" {
		currentExe, err = os.Executable()
		if err != nil {
			return fail(fmt.Errorf("locate current executable: %w", err))
		}
	}
	if err := replaceBinary(currentExe, newBinary); err != nil {
		return fail(err)
	}

	result.SourceDir = sourceDir
	result.BinaryPath = currentExe
	result.RestartRequired = true
	return result, nil
}

func archiveExtension(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".zip") {
		return ".zip"
	}
	return ".tar.gz"
}

func downloadAsset(ctx context.Context, info Info, dest string, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.AssetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", userAgentName+"/"+info.Current)

	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %s", resp.Status)
	}
	if resp.ContentLength > MaxAssetSize {
		return fmt.Errorf("download: asset is %d bytes, larger than the %d byte limit", resp.ContentLength, MaxAssetSize)
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(resp.Body, MaxAssetSize+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > MaxAssetSize {
		return fmt.Errorf("download: asset exceeds the %d byte limit", MaxAssetSize)
	}
	if info.Size > 0 && written != info.Size {
		return fmt.Errorf("download: got %d bytes, expected %d", written, info.Size)
	}
	if digest := strings.TrimSpace(info.Digest); digest != "" {
		if err := verifyDigest(digest, hasher.Sum(nil)); err != nil {
			return err
		}
	}
	return nil
}

func verifyDigest(expected string, actual []byte) error {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return nil
	}
	algorithm, value, ok := strings.Cut(expected, ":")
	if !ok || algorithm != "sha256" {
		// Unknown digest formats are ignored rather than rejecting a release.
		return nil
	}
	got := hex.EncodeToString(actual)
	if got != value {
		return fmt.Errorf("digest mismatch: got sha256:%s, expected sha256:%s", got, value)
	}
	return nil
}

func extractArchive(archivePath, dest string) error {
	lower := strings.ToLower(archivePath)
	if strings.HasSuffix(lower, ".zip") {
		return extractZip(archivePath, dest)
	}
	return extractTarGz(archivePath, dest)
}

func extractTarGz(archivePath, dest string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return fmt.Errorf("invalid size for %s", header.Name)
			}
			total += header.Size
			if total > maxExtractedSize {
				return fmt.Errorf("archive exceeds the %d byte extraction limit", maxExtractedSize)
			}
			if err := writeEntry(target, reader, header.Size, header.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %q", header.Name)
		}
	}
}

func extractZip(archivePath, dest string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	var total int64
	for _, entry := range reader.File {
		target, err := safeJoin(dest, entry.Name)
		if err != nil {
			return err
		}
		if entry.FileInfo().IsDir() || strings.HasSuffix(entry.Name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported symlink entry %q", entry.Name)
		}
		if entry.UncompressedSize64 > maxExtractedSize {
			return fmt.Errorf("archive exceeds the %d byte extraction limit", maxExtractedSize)
		}
		total += int64(entry.UncompressedSize64)
		if total > maxExtractedSize {
			return fmt.Errorf("archive exceeds the %d byte extraction limit", maxExtractedSize)
		}
		src, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeEntry(target, src, int64(entry.UncompressedSize64), entry.Mode().Perm())
		src.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeEntry(target string, src io.Reader, size int64, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(src, size))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != size {
		return fmt.Errorf("short read for %s: got %d of %d bytes", target, written, size)
	}
	return nil
}

// safeJoin joins an archive entry name to root, rejecting absolute paths and
// traversal outside root (zip-slip).
func safeJoin(root, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == "/" {
		return root, nil
	}
	if path.IsAbs(cleaned) || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	if strings.ContainsRune(cleaned, ':') {
		// Reject Windows drive/alternative-stream syntax even on other
		// platforms so an archive cannot behave differently after transfer.
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	target := filepath.Join(root, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	return target, nil
}

func findSourceRoot(extractDir string) (string, error) {
	if hasAddonSource(extractDir) {
		return extractDir, nil
	}
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(extractDir, entry.Name())
		if hasAddonSource(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("archive does not contain addon/TradePost and data-addon sources")
}

func hasAddonSource(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "addon", "TradePost"))
	if err != nil || !info.IsDir() {
		return false
	}
	info, err = os.Stat(filepath.Join(dir, "data-addon"))
	return err == nil && info.IsDir()
}

func findBinary(sourceDir string) (string, error) {
	name := "tradepost"
	if runtime.GOOS == "windows" {
		name = "tradepost.exe"
	}
	for _, dir := range []string{sourceDir, filepath.Dir(sourceDir)} {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("archive does not contain %s", name)
}

// replaceBinary stages the new executable next to the current one and then
// swaps it in. Linux can rename over a running executable. Windows cannot, so
// the running image is renamed aside first; if that swap fails the new binary
// is left at <current>.new.
func replaceBinary(current, newBinary string) error {
	current, err := filepath.Abs(current)
	if err != nil {
		return err
	}
	if _, err := os.Stat(newBinary); err != nil {
		return fmt.Errorf("new executable: %w", err)
	}
	stage := current + ".new"
	_ = os.Remove(stage)
	if err := copyFile(newBinary, stage, 0o755); err != nil {
		return fmt.Errorf("stage new executable at %s: %w", stage, err)
	}
	if err := os.Chmod(stage, 0o755); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		old := current + ".old"
		// A previous update may have left a stale image; it can normally be
		// removed once the process that used it exited.
		_ = os.Remove(old)
		if err := os.Rename(current, old); err != nil {
			return fmt.Errorf("windows: could not move the running executable aside: %w", err)
		}
		if err := os.Rename(stage, current); err != nil {
			_ = os.Rename(old, current)
			return fmt.Errorf("windows: could not install the new executable; left it at %s: %w", stage, err)
		}
		_ = os.Remove(old)
		return nil
	}

	if err := os.Rename(stage, current); err != nil {
		return fmt.Errorf("replace %s: %w (new binary left at %s)", current, err, stage)
	}
	return os.Chmod(current, 0o755)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
