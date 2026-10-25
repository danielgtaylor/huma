// The leak tests rely on RLIMIT_NOFILE, which only exists on unix.
//go:build unix

package huma

import (
	"bytes"
	"mime/multipart"
	"reflect"
	"runtime/debug"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// limitFileDescriptors makes leaked handles fail fast: a low RLIMIT_NOFILE turns
// leaks into EMFILE, and disabling GC stops finalizers from closing them. Leaks
// accumulate, so tests only need to check their last call.
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

// diskBackedForm uses a zero memory threshold so Open returns a real *os.File.
func diskBackedForm(t *testing.T, contentTypes ...string) *multipart.Form {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for range contentTypes {
		part, err := w.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("hello, world!"))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())

	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(0)
	require.NoError(t, err)
	t.Cleanup(func() { form.RemoveAll() })
	for i, fh := range form.File["file"] {
		fh.Header.Set("Content-Type", contentTypes[i])
	}
	return form
}

func TestMimeTypeValidatorClosesFile(t *testing.T) {
	fh := diskBackedForm(t, "text/plain").File["file"][0]
	validator := NewMimeTypeValidator(&Encoding{ContentType: "text/plain"})
	limitFileDescriptors(t)

	var detail *ErrorDetail
	for range 200 {
		_, detail = validator.Validate(fh, "file")
	}

	assert.Nil(t, detail)
}

func TestReadFileClosesOnValidationError(t *testing.T) {
	fh := diskBackedForm(t, "text/plain").File["file"][0]
	validator := NewMimeTypeValidator(&Encoding{ContentType: "image/png"})
	limitFileDescriptors(t)

	var detail *ErrorDetail
	for range 200 {
		_, detail = readFile(fh, "file", validator)
	}

	require.NotNil(t, detail)
	assert.Contains(t, detail.Message, "Invalid mime type")
}

func TestDecodeClosesFilesOnValidationError(t *testing.T) {
	type input struct {
		Files []FormFile `form:"file" contentType:"image/png"`
	}
	registry := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	typ := reflect.TypeFor[input]()
	mediaType := &MediaType{
		Schema:   multiPartFormFileSchema(registry, typ),
		Encoding: multiPartContentEncoding(typ),
	}
	// The first file is opened before the second fails validation.
	form := diskBackedForm(t, "image/png", "text/plain")
	limitFileDescriptors(t)

	var errs []error
	for range 200 {
		m := MultipartFormFiles[input]{Form: form}
		errs = m.Decode(mediaType, func(reflect.Value) {})
	}

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "Invalid mime type")
}
