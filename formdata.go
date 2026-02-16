package huma

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
)

type FormFile struct {
	multipart.File
	ContentType string // Content-Type as declared in the multipart form field, or detected when parsing request as fallback
	IsSet       bool   // Indicates whether content was received when working with optional files
	Size        int64  // File size in bytes
	Filename    string // Filename as declared in the multipart form field, if any
}

type MultipartFormFiles[T any] struct {
	Form *multipart.Form
	data *T
}

func (m *MultipartFormFiles[T]) Data() *T {
	return m.data
}

type MimeTypeValidator struct {
	accept []string
}

func NewMimeTypeValidator(encoding *Encoding) MimeTypeValidator {
	var mimeTypes = strings.Split(encoding.ContentType, ",")
	for i := range mimeTypes {
		mimeTypes[i] = strings.Trim(mimeTypes[i], " ")
	}
	if len(mimeTypes) == 0 {
		mimeTypes = []string{"application/octet-stream"}
	}
	return MimeTypeValidator{accept: mimeTypes}
}

// Validate checks the mime type of the provided file against the expected content type.
// In the absence of a Content-Type file header, the mime type is detected using [http.DetectContentType].
func (v MimeTypeValidator) Validate(fh *multipart.FileHeader, location string) (string, *ErrorDetail) {
	file, err := fh.Open()
	if err != nil {
		return "", &ErrorDetail{Message: "Failed to open file", Location: location}
	}

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" {
		var buffer = make([]byte, 1000)
		if _, err := file.Read(buffer); err != nil {
			return "", &ErrorDetail{Message: "Failed to infer file media type", Location: location}
		}
		file.Seek(int64(0), io.SeekStart)
		mimeType = http.DetectContentType(buffer)
	}
	accept := false
	for _, m := range v.accept {
		if m == "text/plain" || m == "application/octet-stream" {
			accept = true
			break
		}
		if strings.HasSuffix(m, "/*") &&
			strings.HasPrefix(mimeType, strings.TrimRight(m, "*")) {
			accept = true
			break
		}
		if mimeType == m {
			accept = true
			break
		}
	}

	if accept {
		return mimeType, nil
	} else {
		return mimeType, &ErrorDetail{
			Message: fmt.Sprintf(
				"Invalid mime type: got %v, expected %v",
				mimeType, strings.Join(v.accept, ","),
			),
			Location: location,
			Value:    mimeType,
		}
	}
}

// Decodes multipart.Form data into *T, returning []*ErrorDetail if any
// Schema is used to check for validation constraints
func (m *MultipartFormFiles[T]) Decode(opMediaType *MediaType, formValueParser func(val reflect.Value)) []error {
	var (
		dataType = reflect.TypeOf(m.data).Elem()
		value    = reflect.New(dataType)
		errors   []error
	)
	for i := 0; i < dataType.NumField(); i++ {
		field := value.Elem().Field(i)
		structField := dataType.Field(i)
		key := structField.Tag.Get("form")
		if key == "" {
			key = structField.Name
		}
		fileHeaders := m.Form.File[key]
		switch {
		case field.Type() == reflect.TypeOf(FormFile{}):
			file, err := readSingleFile(fileHeaders, key, opMediaType)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			field.Set(reflect.ValueOf(file))
		case field.Type() == reflect.TypeOf([]FormFile{}):
			files, errs := readMultipleFiles(fileHeaders, key, opMediaType)
			if errs != nil {
				errors = append(errors, errs...)
				continue
			}
			field.Set(reflect.ValueOf(files))
		}
	}
	formValueParser(value)
	m.data = value.Interface().(*T)
	return errors
}

func readSingleFile(fileHeaders []*multipart.FileHeader, key string, opMediaType *MediaType) (FormFile, *ErrorDetail) {
	if len(fileHeaders) == 0 {
		if opMediaType.Schema.requiredMap[key] {
			return FormFile{}, &ErrorDetail{Message: "File required", Location: key}
		}
		return FormFile{}, nil
	} else if len(fileHeaders) == 1 {
		validator := NewMimeTypeValidator(opMediaType.Encoding[key])
		return readFile(fileHeaders[0], key, validator)
	}
	return FormFile{}, &ErrorDetail{
		Message:  "Multiple files received but only one was expected",
		Location: key,
	}
}

func readMultipleFiles(fileHeaders []*multipart.FileHeader, key string, opMediaType *MediaType) ([]FormFile, []error) {
	var (
		files  = make([]FormFile, len(fileHeaders))
		errors []error
	)
	if opMediaType.Schema.requiredMap[key] && len(fileHeaders) == 0 {
		return nil, []error{&ErrorDetail{Message: "At least one file is required", Location: key}}
	}
	validator := NewMimeTypeValidator(opMediaType.Encoding[key])
	for i, fh := range fileHeaders {
		file, err := readFile(
			fh,
			fmt.Sprintf("%s[%d]", key, i),
			validator,
		)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		files[i] = file
	}
	return files, errors
}

func readFile(
	fh *multipart.FileHeader,
	location string,
	validator MimeTypeValidator,
) (FormFile, *ErrorDetail) {
	f, err := fh.Open()
	if err != nil {
		return FormFile{}, &ErrorDetail{Message: "Failed to open file", Location: location}
	}
	contentType, validationErr := validator.Validate(fh, location)
	if validationErr != nil {
		return FormFile{}, validationErr
	}
	return FormFile{
		File:        f,
		ContentType: contentType,
		IsSet:       true,
		Size:        fh.Size,
		Filename:    fh.Filename,
	}, nil
}

func formDataFieldName(f reflect.StructField) string {
	name := f.Name
	if formDataKey := f.Tag.Get("form"); formDataKey != "" {
		name = formDataKey
	}
	return name
}

func multiPartFormFileSchema(r Registry, t reflect.Type) *Schema {
	nFields := t.NumField()
	schema := &Schema{
		Type:        "object",
		Properties:  make(map[string]*Schema, nFields),
		requiredMap: make(map[string]bool, nFields),
	}
	requiredFields := make([]string, 0, nFields)
	for i := 0; i < nFields; i++ {
		f := t.Field(i)
		name := formDataFieldName(f)

		switch {
		case f.Type == reflect.TypeOf(FormFile{}):
			schema.Properties[name] = multiPartFileSchema(f)
		case f.Type == reflect.TypeOf([]FormFile{}):
			schema.Properties[name] = &Schema{
				Type:  "array",
				Items: multiPartFileSchema(f),
			}
		default:
			schema.Properties[name] = SchemaFromField(r, f, name)

			// Should we panic if [T] struct defines fields with unsupported types ?
		}

		if _, ok := f.Tag.Lookup("required"); ok && boolTag(f, "required", false) {
			requiredFields = append(requiredFields, name)
			schema.requiredMap[name] = true
		}
	}
	schema.Required = requiredFields
	return schema
}

func multiPartFileSchema(f reflect.StructField) *Schema {
	return &Schema{
		Type:            "string",
		Format:          "binary",
		Description:     f.Tag.Get("doc"),
		ContentEncoding: "binary",
	}
}

func multiPartContentEncoding(t reflect.Type) map[string]*Encoding {
	nFields := t.NumField()
	encoding := make(map[string]*Encoding, nFields)
	for i := 0; i < nFields; i++ {
		f := t.Field(i)
		name := formDataFieldName(f)

		contentType := "text/plain"
		if f.Type == reflect.TypeOf(FormFile{}) || f.Type == reflect.TypeOf([]FormFile{}) {
			contentType = f.Tag.Get("contentType")
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}
		encoding[name] = &Encoding{
			ContentType: contentType,
		}
	}
	return encoding
}
