package jsonrpc

// http://www.jsonrpc.org/specification
const JSONRPCVersion = "2.0"

// RequestID can be a int or a string
// Do a type alias as we want marshal/unmarshal etc to be available
type RequestID = IntString

type Request[T any] struct {
	// Support JSON RPC v2.
	JSONRPC string     `json:"jsonrpc"          enum:"2.0" doc:"JSON-RPC version, must be '2.0'"                                     required:"true"`
	ID      *RequestID `json:"id,omitempty"                doc:"RequestID is int or string for methods and absent for notifications"`
	Method  string     `json:"method"                      doc:"Method to invoke"                                                    required:"true"`
	Params  T          `json:"params,omitempty"            doc:"Method parameters"`
}

type Response[T any] struct {
	JSONRPC string        `json:"jsonrpc"          required:"true"`
	ID      *RequestID    `json:"id,omitempty"`
	Result  T             `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// A notification which does not expect a response.
type Notification[T any] struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  T      `json:"params,omitempty"`
}
