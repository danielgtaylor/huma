package huma

// ErrorModel defines a basic error message
type ErrorModel struct {
	// Type is a URI to get more information about the error type.
	Type string `json:"type,omitempty" format:"uri" default:"about:blank" example:"https://example.com/errors/example" "doc:"A URI reference to human-readable documentation for the error."`
	// Title provides a short static summary of the problem. Huma will default this
	// to the HTTP response status code text if not present.
	Title string `json:"title,omitempty" example:"Bad Request" doc:"A short, human-readable summary of the problem type. This value should not change between occurances of the error."`
	// Status provides the HTTP status code for client convenience. Huma will
	// default this to the response status code if unset. This SHOULD match the
	// response status code (though proxies may modify the actual status code).
	Status int `json:"status,omitempty" example:"400" doc:"HTTP status code"`
	// Detail is an explanation specific to this error occurrence.
	Detail string `json:"detail,omitempty" example:"Property foo is required but is missing." doc:"A human-readable explanation specific to this occurrence of the problem."`
	// Instance is a URI to get more info about this error occurence.
	Instance string `json:"instance,omitempty" format:"uri" example:"https://example.com/error-log/abc123" doc:"A URI reference that identifies the specific occurence of the problem."`
	// Errors provides an optional mechanism of passing additional error detail
	// strings as a list, which tends to display better than a large multi-line
	// string with many errors.
	Errors []string `json:"errors,omitempty" doc:"Optional list of individual error details"`
}
