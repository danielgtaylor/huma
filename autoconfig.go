package huma

// AutoConfigVar represents a variable given by the user when prompted during
// auto-configuration setup of an API.
type AutoConfigVar struct {
	Description string        `json:"description,omitempty"`
	Example     string        `json:"example,omitempty"`
	Default     interface{}   `json:"default,omitempty"`
	Enum        []interface{} `json:"enum,omitempty"`
}

// AutoConfig holds an API's automatic configuration settings for the CLI. These
// are advertised via OpenAPI extension and picked up by the CLI to make it
// easier to get started using an API.
type AutoConfig struct {
	Security string                   `json:"security"`
	Headers  map[string]string        `json:"headers,omitempty"`
	Prompt   map[string]AutoConfigVar `json:"prompt,omitempty"`
	Params   map[string]string        `json:"params"`
}
