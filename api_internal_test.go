package huma

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetServerURLWithDefaultVars(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		vars     map[string]*ServerVariable
		expected string
	}{
		{
			name: "AbsURL",
			url:  "http://localhost:{port}/api/{version}{ext}?{unknown_var}",
			vars: map[string]*ServerVariable{
				"port": {
					Default: "8080",
				},
				"version": {
					Enum: []string{"v1", "v2"},
				},
				"ext": {},
			},
			expected: "http://localhost:8080/api/v1?{unknown_var}",
		},
		{
			name: "RelativeURL",
			url:  "/{basepath}/{version}",
			vars: map[string]*ServerVariable{
				"basepath": {
					Default: "api",
				},
				"version": {
					Default: "v2",
				},
			},
			expected: "/api/v2",
		},
		{
			name:     "EmptyURL",
			url:      "",
			expected: "",
		},
		{
			name:     "NoVars",
			url:      "http://localhost:{port}/api/{version}",
			expected: "http://localhost:{port}/api/{version}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := Server{URL: test.url, Variables: test.vars}
			url := getServerURLWithDefaultVars(server)

			assert.Equal(t, test.expected, url)
		})
	}
}
