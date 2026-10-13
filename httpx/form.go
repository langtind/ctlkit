package httpx

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
)

// Form builds the body of a multipart form, as a web client sends one that
// carries files. Add the parts in the order the service's own client does.
type Form struct {
	body   bytes.Buffer
	writer *multipart.Writer
	err    error
}

// FormFile is one file in a form.
type FormFile struct {
	// Name is the file name the service is told. Give the last element
	// only, never a path on this machine.
	Name string
	// ContentType is the type of Data. Empty means it is read from Data.
	ContentType string
	Data        []byte
}

func (f *Form) form() *multipart.Writer {
	if f.writer == nil {
		f.writer = multipart.NewWriter(&f.body)
	}
	return f.writer
}

// Field adds a text field.
func (f *Form) Field(name, value string) *Form {
	if f.err == nil {
		f.err = f.form().WriteField(name, value)
	}
	return f
}

// JSON adds a field that holds v as JSON, which is how many services take
// the request itself next to its files.
func (f *Form) JSON(name string, v any) *Form {
	data, err := json.Marshal(v)
	if err != nil && f.err == nil {
		f.err = err
	}
	return f.Field(name, string(data))
}

// File adds a file. Several files may share one field name.
func (f *Form) File(field string, file FormFile) *Form {
	if f.err != nil {
		return f
	}
	contentType := file.ContentType
	if contentType == "" {
		contentType = http.DetectContentType(file.Data)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": file.Name}))
	header.Set("Content-Type", contentType)
	part, err := f.form().CreatePart(header)
	if err == nil {
		_, err = part.Write(file.Data)
	}
	f.err = err
	return f
}

// Body closes the form and returns it with the Content-Type header that
// names its boundary. The form takes no more parts after this.
func (f *Form) Body() (body []byte, contentType string, err error) {
	if f.err != nil {
		return nil, "", f.err
	}
	if err := f.form().Close(); err != nil {
		return nil, "", err
	}
	return f.body.Bytes(), f.writer.FormDataContentType(), nil
}
