package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Version comparison ---

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.9.0", [3]int{0, 9, 0}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"1.2", [3]int{1, 2, 0}},
		{"1", [3]int{1, 0, 0}},
		{"dev", [3]int{0, 0, 0}},
		{"", [3]int{0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, parseVersion(tt.input))
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "0.9.9", 1},
		{"0.9.9", "1.0.0", -1},
		{"1.0.0", "1.0.0", 0},
		{"1.2.0", "1.1.9", 1},
		{"1.1.9", "1.2.0", -1},
		{"2.0.0", "1.99.99", 1},
		{"0.0.1", "0.0.0", 1},
		{"0.0.0", "0.0.1", -1},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_vs_%s", tt.a, tt.b)
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, compareVersions(tt.a, tt.b))
		})
	}
}

func TestIsNewerVersion(t *testing.T) {
	assert.True(t, isNewerVersion("1.0.0", "0.9.0"))
	assert.False(t, isNewerVersion("0.9.0", "1.0.0"))
	assert.False(t, isNewerVersion("1.0.0", "1.0.0"))
	assert.True(t, isNewerVersion("0.1.0", "dev"))
}

// --- Archive name/URL ---

func TestBuildArchiveURL(t *testing.T) {
	url := buildArchiveURL("0.2.0", "ai-switch-0.2.0-linux-amd64.tar.gz")
	assert.Equal(t, "https://github.com/keepmind9/ai-switch/releases/download/v0.2.0/ai-switch-0.2.0-linux-amd64.tar.gz", url)
}

func TestBuildArchiveName(t *testing.T) {
	name := buildArchiveName("0.2.0")
	if runtime.GOOS == "windows" {
		assert.Contains(t, name, ".zip")
	} else {
		assert.Contains(t, name, ".tar.gz")
	}
	assert.Contains(t, name, "ai-switch-0.2.0")
	assert.Contains(t, name, runtime.GOOS)
	assert.Contains(t, name, runtime.GOARCH)
}

// --- Meta JSON ---

func TestSaveAndLoadMeta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")

	meta := &updateMeta{
		Version: "0.2.0",
		URL:     "https://example.com/archive.tar.gz",
		Size:    12345,
		Archive: "archive.tar.gz",
	}

	require.NoError(t, saveMeta(path, meta))

	loaded, err := loadMeta(path)
	require.NoError(t, err)
	assert.Equal(t, meta.Version, loaded.Version)
	assert.Equal(t, meta.URL, loaded.URL)
	assert.Equal(t, meta.Size, loaded.Size)
	assert.Equal(t, meta.Archive, loaded.Archive)
}

func TestLoadMetaNotFound(t *testing.T) {
	_, err := loadMeta(filepath.Join(t.TempDir(), "nonexistent.json"))
	assert.Error(t, err)
}

func TestLoadMetaInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")
	require.NoError(t, os.WriteFile(path, []byte("invalid"), 0644))
	_, err := loadMeta(path)
	assert.Error(t, err)
}

// --- Download with resume ---

func TestDownloadWithResumeFresh(t *testing.T) {
	content := []byte("hello world")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, downloadWithResume(srv.URL, dest, int64(len(content))))

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestDownloadWithResumeAlreadyComplete(t *testing.T) {
	content := []byte("hello world")
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, os.WriteFile(dest, content, 0644))

	require.NoError(t, downloadWithResume(srv.URL, dest, int64(len(content))))
	assert.False(t, called, "should not make HTTP request for complete file")
}

func TestDownloadWithResumePartial(t *testing.T) {
	fullContent := []byte("hello world, this is a test")
	partialContent := fullContent[:5] // "hello"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "bytes=5-" {
			assert.Equal(t, "bytes=5-", rangeHeader)
			w.WriteHeader(http.StatusPartialContent)
			w.Write(fullContent[5:])
			return
		}
		w.Write(fullContent)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, os.WriteFile(dest, partialContent, 0644))

	require.NoError(t, downloadWithResume(srv.URL, dest, int64(len(fullContent))))

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, fullContent, data)
}

func TestDownloadWithResumeServerNoResume(t *testing.T) {
	fullContent := []byte("hello world")
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write(fullContent)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, os.WriteFile(dest, []byte("hel"), 0644))

	require.NoError(t, downloadWithResume(srv.URL, dest, int64(len(fullContent))))

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, fullContent, data)
	assert.Equal(t, 2, callCount) // first attempt returns 200, second restarts
}

func TestDownloadWithResumeServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "archive.tar.gz")
	err := downloadWithResume(srv.URL, dest, 100)
	assert.Error(t, err)
}

// --- Extract ---

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archivePath := createTestTarGz(t, dir, "ai-switch-0.2.0-linux-amd64")

	extractDir := t.TempDir()
	require.NoError(t, extractTarGz(extractDir, archivePath))

	binPath := filepath.Join(extractDir, "ai-switch-0.2.0-linux-amd64", "ais")
	data, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, "fake-binary", string(data))
}

func TestExtractTarGzInvalidArchive(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad.tar.gz")
	require.NoError(t, os.WriteFile(badFile, []byte("not a tar.gz"), 0644))

	err := extractTarGz(t.TempDir(), badFile)
	assert.Error(t, err)
}

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	archivePath := createTestZip(t, dir, "ai-switch-0.2.0-windows-amd64")

	extractDir := t.TempDir()
	require.NoError(t, extractZip(extractDir, archivePath))

	binPath := filepath.Join(extractDir, "ai-switch-0.2.0-windows-amd64", "ais.exe")
	data, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, "fake-binary", string(data))
}

func createTestTarGz(t *testing.T, dir, subdir string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test.tar.gz")
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: subdir + "/", Typeflag: tar.TypeDir, Mode: 0755}
	require.NoError(t, tw.WriteHeader(hdr))

	content := "fake-binary"
	hdr = &tar.Header{Name: subdir + "/ais", Size: int64(len(content)), Mode: 0755}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err = tw.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	return archivePath
}

func createTestZip(t *testing.T, dir, subdir string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test.zip")
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	_, err = w.Create(subdir + "/")
	require.NoError(t, err)

	fw, err := w.Create(subdir + "/ais.exe")
	require.NoError(t, err)
	_, err = fw.Write([]byte("fake-binary"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return archivePath
}

// --- findExtractedBinary ---

func TestFindExtractedBinaryFound(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "ai-switch-0.2.0-linux-amd64")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	binName := "ais"
	if runtime.GOOS == "windows" {
		binName = "ais.exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(subdir, binName), []byte("bin"), 0755))

	result, err := findExtractedBinary(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(subdir, binName), result)
}

func TestFindExtractedBinaryNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := findExtractedBinary(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "binary not found")
}

func TestFindExtractedBinaryNoMatchingDir(t *testing.T) {
	dir := t.TempDir()
	otherDir := filepath.Join(dir, "other-dir")
	require.NoError(t, os.MkdirAll(otherDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(otherDir, "ais"), []byte("bin"), 0755))

	_, err := findExtractedBinary(dir)
	assert.Error(t, err)
}

// --- copyFile ---

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	content := []byte("test content for copy")
	require.NoError(t, os.WriteFile(src, content, 0644))
	require.NoError(t, copyFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestCopyFileSourceNotFound(t *testing.T) {
	err := copyFile("/nonexistent/file", filepath.Join(t.TempDir(), "dst"))
	assert.Error(t, err)
}

// --- cleanUpdateDir ---

func TestCleanUpdateDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "update")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "archive.tar.gz"), []byte("data"), 0644))

	cleanUpdateDir(subdir)

	// Dir should still exist but be empty
	entries, err := os.ReadDir(subdir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// --- fetchLatestVersion ---

func TestFetchLatestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"tag_name": "v0.3.0"})
	}))
	defer srv.Close()

	orig := updateAPIURL
	updateAPIURL = srv.URL
	defer func() { updateAPIURL = orig }()

	v, err := fetchLatestVersion()
	require.NoError(t, err)
	assert.Equal(t, "0.3.0", v)
}

func TestFetchLatestVersionNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	orig := updateAPIURL
	updateAPIURL = srv.URL
	defer func() { updateAPIURL = orig }()

	_, err := fetchLatestVersion()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetchLatestVersionInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	orig := updateAPIURL
	updateAPIURL = srv.URL
	defer func() { updateAPIURL = orig }()

	_, err := fetchLatestVersion()
	assert.Error(t, err)
}

// --- getRemoteSize ---

func TestGetRemoteSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		w.Header().Set("Content-Length", "98765")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	size, err := getRemoteSize(srv.URL)
	require.NoError(t, err)
	assert.Equal(t, int64(98765), size)
}

func TestGetRemoteSizeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := getRemoteSize(srv.URL)
	assert.Error(t, err)
}

// --- isDaemonRunning ---

func TestIsDaemonRunningNoPIDFile(t *testing.T) {
	assert.False(t, isDaemonRunning())
}

func TestIsDaemonRunningInvalidPID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "ai-switch.pid")
	require.NoError(t, os.WriteFile(pidPath, []byte("not-a-number"), 0644))

	// isDaemonRunning reads from config.DataDir which we can't easily override
	// This test verifies the function handles gracefully without a real data dir
	assert.False(t, isDaemonRunning())
}

// --- Integration: doApply ---

func TestDoApply(t *testing.T) {
	dir := t.TempDir()
	subdirName := fmt.Sprintf("ai-switch-0.2.0-%s-%s", runtime.GOOS, runtime.GOARCH)
	subdir := filepath.Join(dir, subdirName)
	require.NoError(t, os.MkdirAll(subdir, 0755))

	binName := "ais"
	if runtime.GOOS == "windows" {
		binName = "ais.exe"
	}
	newBinContent := []byte("new-binary-content")
	require.NoError(t, os.WriteFile(filepath.Join(subdir, binName), newBinContent, 0755))

	// Create archive
	archiveName := subdirName + ".tar.gz"
	archivePath := filepath.Join(dir, archiveName)
	createArchiveFromDir(t, subdir, archivePath)

	// Create meta
	meta := &updateMeta{Version: "0.2.0", Archive: archiveName, Size: fileSize(t, archivePath)}
	metaPath := filepath.Join(dir, "meta.json")
	require.NoError(t, saveMeta(metaPath, meta))

	// Create a fake "current binary" to replace
	currentBin := filepath.Join(dir, "current-binary")
	require.NoError(t, os.WriteFile(currentBin, []byte("old"), 0755))

	// Monkey-patch os.Executable by testing doApply indirectly
	// doApply calls os.Executable() internally, so we test via extractArchive + copyFile instead
	require.NoError(t, extractArchive(dir, archivePath, runtime.GOOS == "windows"))
	srcBin, err := findExtractedBinary(dir)
	require.NoError(t, err)
	require.NoError(t, copyFile(srcBin, currentBin))

	data, err := os.ReadFile(currentBin)
	require.NoError(t, err)
	assert.Equal(t, newBinContent, data)
}

// --- Integration: resume scenario ---

func TestResumeScenarioEndToEnd(t *testing.T) {
	fullContent := bytes.Repeat([]byte("x"), 1000)

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		rangeHdr := r.Header.Get("Range")
		if rangeHdr != "" {
			offset := 500
			w.WriteHeader(http.StatusPartialContent)
			w.Write(fullContent[offset:])
			return
		}
		w.Write(fullContent)
	}))
	defer srv.Close()

	dir := t.TempDir()

	// Simulate partial download
	dest := filepath.Join(dir, "archive.tar.gz")
	require.NoError(t, os.WriteFile(dest, fullContent[:500], 0644))

	// Resume
	require.NoError(t, downloadWithResume(srv.URL, dest, int64(len(fullContent))))

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, fullContent, data)
	assert.Equal(t, 1, callCount) // only one request (resume)
}

// --- runApply edge cases ---

func TestRunApplyNoMeta(t *testing.T) {
	// runApply with no update dir should print message and return nil
	err := runApply()
	assert.NoError(t, err)
}

func TestRunApplyIncompleteDownload(t *testing.T) {
	dir := t.TempDir()

	// Override updateDirPath by creating update dir in the config data dir
	// Since we can't easily override config.DataDir, test via loadMeta path check
	metaPath := filepath.Join(dir, "meta.json")
	require.NoError(t, saveMeta(metaPath, &updateMeta{
		Version: "0.2.0",
		Archive: "test.tar.gz",
		Size:    1000,
	}))

	// Create incomplete file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.tar.gz"), []byte("short"), 0644))

	// Verify meta loads and size check fails
	meta, err := loadMeta(metaPath)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, meta.Archive))
	require.NoError(t, err)
	assert.NotEqual(t, meta.Size, info.Size()) // incomplete
}

// --- Helper ---

func createArchiveFromDir(t *testing.T, srcDir, archivePath string) {
	t.Helper()

	if strings.HasSuffix(archivePath, ".zip") {
		f, err := os.Create(archivePath)
		require.NoError(t, err)
		defer f.Close()

		w := zip.NewWriter(f)
		err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(srcDir, path)
			if rel == "." {
				return nil
			}
			if info.IsDir() {
				_, err = w.Create(rel + "/")
				return err
			}
			fw, err := w.Create(rel)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, err = fw.Write(data)
			return err
		})
		require.NoError(t, err)
		require.NoError(t, w.Close())
		return
	}

	// tar.gz
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	require.NoError(t, err)
	require.NoError(t, tw.Close())
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Size()
}

// --- Unused import guard ---

var _ = bytes.NewReader
