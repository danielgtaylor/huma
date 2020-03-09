package huma

// ErrorModel defines a basic error message
type ErrorModel struct {
	Message string `json:"message"`
}

// ErrorInvalidModel defines an HTTP 400 Invalid response message
type ErrorInvalidModel struct {
	Message string   `json:"message"`
	Errors  []string `json:"errors"`
}
