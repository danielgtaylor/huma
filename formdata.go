package huma

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

type MultipartFormFiles[T any] struct {
	Form *multipart.Form
	data *T
}

type MimeTypeValidator struct {
	accept []string
}

var contentTypeSplitRe = regexp.MustCompile(`,\s*`)

func NewMimeTypeValidator(encoding *Encoding) MimeTypeValidator {
	return MimeTypeValidator{accept: contentTypeSplitRe.Split(encoding.ContentType, -1)}
}

func (v MimeTypeValidator) Validate(file multipart.File, location string) *ErrorDetail {
	var buffer = make([]byte, 1000)
	if _, err := file.Read(buffer); err != nil {
		return &ErrorDetail{Message: "Failed to infer file media type", Location: location}
	}
	file.Seek(int64(0), io.SeekStart)

	mimeType := http.DetectContentType(buffer)
	accept := slices.ContainsFunc(v.accept, func(m string) bool {
		if strings.HasSuffix(m, "/*") &&
			strings.HasPrefix(mimeType, strings.TrimRight(m, "*")) {
			return true
		}
		if mimeType == m {
			return true
		}
		return false
	})

	if accept {
		return nil
	} else {
		return &ErrorDetail{
			Message: fmt.Sprintf(
				"Invalid mime type, expected %v",
				strings.Join(v.accept, ","),
			),
			Location: location,
			Value:    mimeType,
		}
	}
}

func (m *MultipartFormFiles[T]) readFile(
	fh *multipart.FileHeader,
	location string,
	validator MimeTypeValidator,
) (multipart.File, *ErrorDetail) {
	f, err := fh.Open()
	if err != nil {
		return nil, &ErrorDetail{Message: "Failed to open file", Location: location}
	}
	if validationErr := validator.Validate(f, location); validationErr != nil {
		return nil, validationErr
	}
	return f, nil
}

func (m *MultipartFormFiles[T]) readSingleFile(key string, opMediaType *MediaType) (multipart.File, *ErrorDetail) {
	fileHeaders := m.Form.File[key]
	if len(fileHeaders) == 0 {
		if opMediaType.Schema.requiredMap[key] {
			return nil, &ErrorDetail{Message: "File required", Location: key}
		} else {
			return nil, nil
		}
	} else if len(fileHeaders) == 1 {
		validator := NewMimeTypeValidator(opMediaType.Encoding[key])
		return m.readFile(fileHeaders[0], key, validator)
	}
	return nil, &ErrorDetail{
		Message:  "Multiple files received but only one was expected",
		Location: key,
	}
}

func (m *MultipartFormFiles[T]) readMultipleFiles(key string, opMediaType *MediaType) ([]multipart.File, []error) {
	fileHeaders := m.Form.File[key]
	var (
		files  = make([]multipart.File, len(fileHeaders))
		errors []error
	)
	if opMediaType.Schema.requiredMap[key] && len(fileHeaders) == 0 {
		return nil, []error{&ErrorDetail{Message: "At least one file is required", Location: key}}
	}
	validator := NewMimeTypeValidator(opMediaType.Encoding[key])
	for i, fh := range fileHeaders {
		file, err := m.readFile(
			fh,
			fmt.Sprintf("%s[%d]", key, i),
			validator,
		)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		files = append(files, file)
	}
	return files, errors
}

func (m *MultipartFormFiles[T]) Data() *T {
	return m.data
}

// Decodes multipart.Form data into *T, returning []*ErrorDetail if any
// Schema is used to check for validation constraints
func (m *MultipartFormFiles[T]) Decode(opMediaType *MediaType) []error {
	var (
		dataType = reflect.TypeOf(m.data).Elem()
		value    = reflect.New(dataType)
		errors   []error
	)
	for i := 0; i < dataType.NumField(); i++ {
		field := value.Elem().Field(i)
		key := dataType.Field(i).Tag.Get("form-data")
		if key == "" {
			continue
		}
		switch {
		case field.Type().String() == "multipart.File":
			file, err := m.readSingleFile(key, opMediaType)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			field.Set(reflect.ValueOf(file))
		case field.Type().String() == "[]multipart.File":
			files, errs := m.readMultipleFiles(key, opMediaType)
			if errs != nil {
				errors = slices.Concat(errors, errs)
				continue
			}
			field.Set(reflect.ValueOf(files))

		default:
			continue
		}
	}
	m.data = value.Interface().(*T)
	return errors
}

func formDataFieldName(f reflect.StructField) string {
	name := f.Name
	if formDataKey := f.Tag.Get("form-data"); formDataKey != "" {
		name = formDataKey
	}
	return name
}

func multiPartFormFileSchema(t reflect.Type) *Schema {
	nFields := t.NumField()
	schema := &Schema{
		Type:        "object",
		Properties:  make(map[string]*Schema, nFields),
		requiredMap: make(map[string]bool, nFields),
	}
	requiredFields := make([]string, nFields)
	for i := 0; i < nFields; i++ {
		f := t.Field(i)
		name := formDataFieldName(f)

		switch {
		case f.Type.String() == "multipart.File":
			schema.Properties[name] = multiPartFileSchema(f)
		case f.Type.String() == "[]multipart.File":
			schema.Properties[name] = &Schema{
				Type:  "array",
				Items: multiPartFileSchema(f),
			}
		default:
			// Should we panic if [T] struct defines fields with unsupported types ?
			continue
		}

		if _, ok := f.Tag.Lookup("required"); ok && boolTag(f, "required") {
			requiredFields[i] = name
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
		encoding[name] = &Encoding{
			ContentType: f.Tag.Get("content-type"),
		}
	}
	return encoding
}
