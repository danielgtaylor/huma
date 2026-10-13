package huma

import (
	"bytes"
	"mime/multipart"
	"runtime/debug"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// limitFileDescriptors lowers the process soft RLIMIT_NOFILE and disables the
// garbage collector for the duration of the test. This makes leaked file
// handles observable: the GC finalizer would otherwise close unreachable
// handles and a high descriptor limit would absorb small leaks. The previous
// values are restored via t.Cleanup.
func limitFileDescriptors(t *testing.T) {
	t.Helper()
	var rl syscall.Rlimit
	require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl))
	orig := rl
	if rl.Cur > 80 {
		rl.Cur = 80
	}
	require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rl))
	origGC := debug.SetGCPercent(-1)
	t.Cleanup(func() {
		debug.SetGCPercent(origGC)
		syscall.Setrlimit(syscall.RLIMIT_NOFILE, &orig)
	})
}

// diskBackedFileHeader parses a one-file-part multipart body with a zero-byte
// memory threshold, so the part is stored in a temporary file on disk and each
// FileHeader.Open returns a real *os.File. The returned form must be cleaned
// up with RemoveAll.
func diskBackedFileHeader(t *testing.T, contentType string) (*multipart.Form, *multipart.FileHeader) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "test.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("hello, world!"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(0)
	require.NoError(t, err)
	fh := form.File["file"][0]
	fh.Header.Set("Content-Type", contentType)
	return form, fh
}

func TestMimeTypeValidatorClosesFile(t *testing.T) {
	form, fh := diskBackedFileHeader(t, "text/plain")
	defer form.RemoveAll()
	limitFileDescriptors(t)

	validator := NewMimeTypeValidator(&Encoding{ContentType: "text/plain"})
	for range 200 {
		_, detail := validator.Validate(fh, "file")
		// A leaked handle per call exhausts the descriptor limit within the
		// loop; with the fix every handle is closed and this never fails.
		require.Nil(t, detail, "Validate unexpectedly failed: %v", detail)
	}
}

func TestReadFileClosesOnValidationError(t *testing.T) {
	form, fh := diskBackedFileHeader(t, "text/plain")
	defer form.RemoveAll()
	limitFileDescriptors(t)

	validator := NewMimeTypeValidator(&Encoding{ContentType: "image/png"})
	for range 200 {
		_, detail := readFile(fh, "file", validator)
		// A leaked handle per call exhausts the descriptor limit within the
		// loop and surfaces as "Failed to open file" instead of the expected
		// mime-type error; with the fix every handle is closed and this never
		// happens.
		require.NotNil(t, detail)
		require.Contains(t, detail.Message, "Invalid mime type")
	}
}
