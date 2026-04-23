package client

import (
	"io"

	"github.com/go-resty/resty/v2"
)

// RequestOption configures a single request.
type RequestOption func(*requestOptions)

type requestOptions struct {
	headers       map[string]string
	queryParams   map[string]string
	pathParams    map[string]string
	body          any
	formData      map[string]string
	files         []fileUpload
	rawBody       []byte
	rawReader     io.Reader
	bindTarget    any
	errorTarget   any
	expectSuccess bool
	contentType   string
	forceJSON     bool
	forceXML      bool
}

type fileUpload struct {
	param    string
	filename string
	reader   io.Reader
}

// --- Request Option constructors ---

// Header sets a single header on this request (overrides client-level defaults).
func Header(key, value string) RequestOption {
	return func(ro *requestOptions) {
		if ro.headers == nil {
			ro.headers = make(map[string]string)
		}
		ro.headers[key] = value
	}
}

// Headers sets multiple headers on this request.
func Headers(headers map[string]string) RequestOption {
	return func(ro *requestOptions) {
		if ro.headers == nil {
			ro.headers = make(map[string]string)
		}
		for k, v := range headers {
			ro.headers[k] = v
		}
	}
}

// QueryParam adds a single query parameter.
func QueryParam(key, value string) RequestOption {
	return func(ro *requestOptions) {
		if ro.queryParams == nil {
			ro.queryParams = make(map[string]string)
		}
		ro.queryParams[key] = value
	}
}

// QueryParams sets multiple query parameters.
func QueryParams(params map[string]string) RequestOption {
	return func(ro *requestOptions) {
		if ro.queryParams == nil {
			ro.queryParams = make(map[string]string)
		}
		for k, v := range params {
			ro.queryParams[k] = v
		}
	}
}

// PathParam sets a URL path parameter, e.g. "/users/{id}" → "/users/42".
func PathParam(key, value string) RequestOption {
	return func(ro *requestOptions) {
		if ro.pathParams == nil {
			ro.pathParams = make(map[string]string)
		}
		ro.pathParams[key] = value
	}
}

// PathParams sets multiple URL path parameters at once.
func PathParams(params map[string]string) RequestOption {
	return func(ro *requestOptions) {
		if ro.pathParams == nil {
			ro.pathParams = make(map[string]string)
		}
		for k, v := range params {
			ro.pathParams[k] = v
		}
	}
}

// Body sets the request body. Accepts any value that can be JSON-marshaled,
// a string, []byte, or an io.Reader.
func Body(v any) RequestOption {
	return func(ro *requestOptions) { ro.body = v }
}

// RawBody sets the request body as raw bytes.
func RawBody(b []byte) RequestOption {
	return func(ro *requestOptions) { ro.rawBody = b }
}

// BodyReader sets the request body from an io.Reader.
func BodyReader(r io.Reader) RequestOption {
	return func(ro *requestOptions) { ro.rawReader = r }
}

// FormData sets the body as application/x-www-form-urlencoded.
func FormData(data map[string]string) RequestOption {
	return func(ro *requestOptions) { ro.formData = data }
}

// File adds a multipart file upload.
// param is the form field name, filename is the file name, reader provides the content.
func File(param, filename string, reader io.Reader) RequestOption {
	return func(ro *requestOptions) {
		ro.files = append(ro.files, fileUpload{param: param, filename: filename, reader: reader})
	}
}

// Bind sets the target to decode a successful JSON response into.
// The pointer is populated automatically after a successful request.
func Bind(v any) RequestOption {
	return func(ro *requestOptions) { ro.bindTarget = v }
}

// BindError sets the target to decode an error JSON response body into.
func BindError(v any) RequestOption {
	return func(ro *requestOptions) { ro.errorTarget = v }
}

// ExpectSuccess causes the request to return an *HTTPError if the status code
// is not in the 2xx range.
func ExpectSuccess() RequestOption {
	return func(ro *requestOptions) { ro.expectSuccess = true }
}

// ContentType sets the Content-Type header for this request.
func ContentType(ct string) RequestOption {
	return func(ro *requestOptions) { ro.contentType = ct }
}

// ForceJSON forces the request body to be encoded as JSON
// and sets Content-Type: application/json.
func ForceJSON() RequestOption {
	return func(ro *requestOptions) { ro.forceJSON = true }
}

// ForceXML forces the request body to be encoded as XML
// and sets Content-Type: application/xml.
func ForceXML() RequestOption {
	return func(ro *requestOptions) { ro.forceXML = true }
}

// applyRequestOptions applies all accumulated request options to the resty.Request.
func applyRequestOptions(req *resty.Request, ro *requestOptions) {
	if len(ro.headers) > 0 {
		req.SetHeaders(ro.headers)
	}
	if len(ro.queryParams) > 0 {
		req.SetQueryParams(ro.queryParams)
	}
	if len(ro.pathParams) > 0 {
		req.SetPathParams(ro.pathParams)
	}
	if ro.body != nil {
		req.SetBody(ro.body)
	}
	if ro.rawBody != nil {
		req.SetBody(ro.rawBody)
	}
	if ro.rawReader != nil {
		req.SetBody(ro.rawReader)
	}
	if len(ro.formData) > 0 {
		req.SetFormData(ro.formData)
	}
	for _, f := range ro.files {
		req.SetFileReader(f.param, f.filename, f.reader)
	}
	if ro.contentType != "" {
		req.SetHeader("Content-Type", ro.contentType)
	}
	if ro.forceJSON {
		req.ForceContentType("application/json")
	}
	if ro.forceXML {
		req.ForceContentType("application/xml")
	}
	if ro.errorTarget != nil {
		req.SetError(ro.errorTarget)
	}
}
