package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    int
	}{
		{"v0.2.4", "0.2.4", 0},
		{"0.2.10", "0.2.4", 1},
		{"0.2.4", "0.2.10", -1},
		{"v0.2.4-dirty", "0.2.4", 0},
		{"0.2.4", "0.2.3", 1},
		{"0.2.3", "0.2.4", -1},
		{"1.0", "0.9.9", 1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.latest, tc.current); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.latest, tc.current, got, tc.want)
		}
	}
	if !IsNewer("v0.2.10", "0.2.4") {
		t.Error("expected v0.2.10 to be newer than 0.2.4")
	}
	if IsNewer("0.2.4", "0.2.4-dirty") {
		t.Error("equal versions must not be newer")
	}
}

func TestSelectAsset(t *testing.T) {
	assets := []Asset{
		{Name: "tradepost-0.2.4-linux-amd64.tar.gz", DownloadURL: "https://example/linux"},
		{Name: "tradepost-companion-0.2.4-windows-amd64.zip", DownloadURL: "https://example/windows"},
	}
	linux, err := SelectAsset(assets, "linux", "amd64")
	if err != nil || linux.DownloadURL != "https://example/linux" {
		t.Fatalf("linux asset = %+v, %v", linux, err)
	}
	windows, err := SelectAsset(assets, "windows", "amd64")
	if err != nil || windows.DownloadURL != "https://example/windows" {
		t.Fatalf("windows asset = %+v, %v", windows, err)
	}
	if _, err := SelectAsset(assets, "darwin", "arm64"); err == nil {
		t.Fatal("expected an error for an unsupported platform")
	}
	if _, err := SelectAsset(assets, "linux", "arm64"); err == nil {
		t.Fatal("expected an error when the matching asset is missing")
	}
}

func TestVerifyDigest(t *testing.T) {
	data := []byte("hello tradepost")
	sum := sha256.Sum256(data)
	good := "sha256:" + hex.EncodeToString(sum[:])
	if err := verifyDigest(good, sum[:]); err != nil {
		t.Fatalf("verifyDigest good: %v", err)
	}
	bad := "sha256:" + hex.EncodeToString(bytes.Repeat([]byte{0}, 32))
	if err := verifyDigest(bad, sum[:]); err == nil {
		t.Fatal("expected digest mismatch")
	}
	if err := verifyDigest("", sum[:]); err != nil {
		t.Fatalf("empty digest should be ignored: %v", err)
	}
}

func TestSafeJoinRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../evil", "addon/../../evil", "/etc/passwd"} {
		if _, err := safeJoin(root, name); err == nil {
			t.Errorf("safeJoin(%q) did not reject traversal", name)
		}
	}
	target, err := safeJoin(root, "addon/TradePost/ok.txt")
	if err != nil {
		t.Fatalf("safeJoin normal path: %v", err)
	}
	if want := filepath.Join(root, "addon", "TradePost", "ok.txt"); target != want {
		t.Errorf("safeJoin = %q, want %q", target, want)
	}
}

func TestExtractTarGz(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "fake.tar.gz")
	writeTarGz(t, archive, map[string]string{
		"tradepost":                          "binary",
		"addon/TradePost/TradePost.toc":      "## Interface: 16001",
		"data-addon/TradePost_Data/Data.lua": "TRADEPOST_DATA = {}",
	})
	dest := t.TempDir()
	if err := extractArchive(archive, dest); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	assertFile(t, filepath.Join(dest, "tradepost"), "binary")
	assertFile(t, filepath.Join(dest, "addon", "TradePost", "TradePost.toc"), "## Interface: 16001")
	assertFile(t, filepath.Join(dest, "data-addon", "TradePost_Data", "Data.lua"), "TRADEPOST_DATA = {}")
}

func TestExtractZip(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "fake.zip")
	writeZip(t, archive, map[string]string{
		"tradepost.exe":                      "binary",
		"addon/TradePost/TradePost.toc":      "## Interface: 16001",
		"data-addon/TradePost_Data/Data.lua": "TRADEPOST_DATA = {}",
	})
	dest := t.TempDir()
	if err := extractArchive(archive, dest); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	assertFile(t, filepath.Join(dest, "tradepost.exe"), "binary")
	assertFile(t, filepath.Join(dest, "addon", "TradePost", "TradePost.toc"), "## Interface: 16001")
}

func TestApplyReplacesBinary(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "fake.tar.gz")
	writeTarGz(t, archive, map[string]string{
		"tradepost":                          "new binary",
		"addon/TradePost/TradePost.toc":      "## Interface: 16001",
		"data-addon/TradePost_Data/Data.lua": "TRADEPOST_DATA = {}",
	})
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Errorf("missing User-Agent")
		}
		w.Write(data)
	}))
	defer server.Close()

	dir := t.TempDir()
	current := filepath.Join(dir, "tradepost")
	if err := os.WriteFile(current, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	info := Info{
		Current:   "0.2.4",
		Latest:    "0.2.5",
		AssetName: "tradepost-0.2.5-linux-amd64.tar.gz",
		AssetURL:  server.URL,
		Digest:    "sha256:" + hex.EncodeToString(sum[:]),
		Size:      int64(len(data)),
	}
	result, err := Apply(context.Background(), info, ApplyOptions{CurrentExe: current})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	defer result.Cleanup()

	assertFile(t, current, "new binary")
	assertFile(t, filepath.Join(result.SourceDir, "addon", "TradePost", "TradePost.toc"), "## Interface: 16001")
	if !result.RestartRequired {
		t.Error("expected RestartRequired")
	}
	if result.Version != "0.2.5" {
		t.Errorf("Version = %q, want 0.2.5", result.Version)
	}
	if _, err := os.Stat(result.SourceDir); err != nil {
		t.Fatalf("source dir should remain until Cleanup: %v", err)
	}
	if err := result.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(result.SourceDir); !os.IsNotExist(err) {
		t.Errorf("Cleanup left source dir behind: %v", err)
	}
}

func TestApplyRejectsBadDigest(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "fake.tar.gz")
	writeTarGz(t, archive, map[string]string{
		"tradepost":                          "new binary",
		"addon/TradePost/TradePost.toc":      "x",
		"data-addon/TradePost_Data/Data.lua": "y",
	})
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer server.Close()

	dir := t.TempDir()
	current := filepath.Join(dir, "tradepost")
	if err := os.WriteFile(current, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := Info{
		Current:   "0.2.4",
		Latest:    "0.2.5",
		AssetName: "tradepost-0.2.5-linux-amd64.tar.gz",
		AssetURL:  server.URL,
		Digest:    "sha256:" + hex.EncodeToString(bytes.Repeat([]byte{0}, 32)),
	}
	if _, err := Apply(context.Background(), info, ApplyOptions{CurrentExe: current}); err == nil {
		t.Fatal("expected digest mismatch error")
	}
	assertFile(t, current, "old binary")
}

func writeTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for name, content := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o755)
		entry, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}
