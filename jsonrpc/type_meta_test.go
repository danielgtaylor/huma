package jsonrpc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// MyData is a sample data structure for testing.
type MyData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// Test unmarshalMeta with Request[json.RawMessage]
func TestUnmarshalMeta_Request(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantIsBatch bool
		wantItems   []Request[json.RawMessage]
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:       "Empty input data",
			data:       []byte{},
			wantErr:    true,
			wantErrMsg: "Received empty data",
		},
		{
			name:        "Valid single request",
			data:        []byte(`{"jsonrpc": "2.0", "method": "sum", "params": [1,2,3], "id":1}`),
			wantIsBatch: false,
			wantItems: []Request[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Method:  "sum",
					Params:  json.RawMessage(`[1,2,3]`),
					ID:      &RequestID{Value: 1},
				},
			},
			wantErr: false,
		},
		{
			name: "Valid batch requests",
			data: []byte(
				`[{"jsonrpc": "2.0", "method": "sum", "params": [1,2,3], "id":1}, {"jsonrpc": "2.0", "method": "subtract", "params": [42,23], "id":2}]`,
			),
			wantIsBatch: true,
			wantItems: []Request[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Method:  "sum",
					Params:  json.RawMessage(`[1,2,3]`),
					ID:      &RequestID{Value: 1},
				},
				{
					JSONRPC: "2.0",
					Method:  "subtract",
					Params:  json.RawMessage(`[42,23]`),
					ID:      &RequestID{Value: 2},
				},
			},
			wantErr: false,
		},
		{
			name:       "Invalid JSON",
			data:       []byte(`{this is not valid JSON}`),
			wantErr:    true,
			wantErrMsg: "Failed to unmarshal single item",
		},
		{
			name:        "Empty batch",
			data:        []byte(`[]`),
			wantErr:     false,
			wantIsBatch: true,
			wantItems:   []Request[json.RawMessage]{},
		},
		{
			name:       "Null input",
			data:       []byte(`null`),
			wantErr:    true,
			wantErrMsg: "Received null data",
		},
		{
			name:       "No input",
			data:       []byte(``),
			wantErr:    true,
			wantErrMsg: "Received empty data",
		},
		{
			name:       "Garbage data",
			data:       []byte(`garbage data`),
			wantErr:    true,
			wantErrMsg: "Failed to unmarshal single item",
		},
		{
			name:       "Whitespace input",
			data:       []byte("   "),
			wantErr:    true,
			wantErrMsg: "Received empty data",
		},
		{
			name:       "Only null byte",
			data:       []byte("\x00"),
			wantErr:    true,
			wantErrMsg: "Failed to unmarshal single item",
		},
		{
			name:       "Array with null",
			data:       []byte(`[null]`),
			wantErr:    true,
			wantErrMsg: "Received null data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var isBatch bool
			var items []Request[json.RawMessage]
			err := unmarshalMeta(tt.data, &isBatch, &items)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unmarshalMeta() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"unmarshalMeta() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}
			if isBatch != tt.wantIsBatch {
				t.Errorf("unmarshalMeta() isBatch = %v, want %v", isBatch, tt.wantIsBatch)
			}
			if !compareRequestSlices(items, tt.wantItems) {
				t.Errorf("unmarshalMeta() items = %+v, want %+v", items, tt.wantItems)
			}
		})
	}
}

// Test marshalMeta with Request[json.RawMessage]
func TestMarshalMeta_Request(t *testing.T) {
	tests := []struct {
		name       string
		isBatch    bool
		items      []Request[json.RawMessage]
		wantData   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "Single item",
			isBatch: false,
			items: []Request[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Method:  "subtract",
					Params:  json.RawMessage(`[42,23]`),
					ID:      &RequestID{Value: 1},
				},
			},
			wantData: `{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}`,
			wantErr:  false,
		},
		{
			name:    "Batch items",
			isBatch: true,
			items: []Request[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Method:  "sum",
					Params:  json.RawMessage(`[1,2,3]`),
					ID:      &RequestID{Value: 1},
				},
				{
					JSONRPC: "2.0",
					Method:  "subtract",
					Params:  json.RawMessage(`[42,23]`),
					ID:      &RequestID{Value: 2},
				},
			},
			wantData: `[{"jsonrpc":"2.0","method":"sum","params":[1,2,3],"id":1},{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":2}]`,
			wantErr:  false,
		},
		{
			name:       "Empty items with isBatch=false",
			isBatch:    false,
			items:      []Request[json.RawMessage]{},
			wantErr:    true,
			wantErrMsg: "Received empty input",
		},
		{
			name:     "Empty items with isBatch=true",
			isBatch:  true,
			items:    []Request[json.RawMessage]{},
			wantData: `[]`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := marshalMeta(tt.isBatch, tt.items)
			if (err != nil) != tt.wantErr {
				t.Fatalf("marshalMeta() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"marshalMeta() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}
			if !jsonStringsEqual(string(data), tt.wantData) {
				t.Errorf("marshalMeta() data = %s, want %s", string(data), tt.wantData)
			}

		})
	}
}

// Test Meta[T] UnmarshalJSON and MarshalJSON with MyData
func TestMeta_MyData(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		wantIsBatch bool
		wantItems   []MyData
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "Single item",
			jsonData:    `{"name": "Item1", "value": 100}`,
			wantIsBatch: false,
			wantItems: []MyData{
				{Name: "Item1", Value: 100},
			},
			wantErr: false,
		},
		{
			name:        "Batch items",
			jsonData:    `[{"name": "Item1", "value": 100}, {"name": "Item2", "value": 200}]`,
			wantIsBatch: true,
			wantItems: []MyData{
				{Name: "Item1", Value: 100},
				{Name: "Item2", Value: 200},
			},
			wantErr: false,
		},
		{
			name:       "Invalid JSON",
			jsonData:   `{"name": "Item1", "value": 100`,
			wantErr:    true,
			wantErrMsg: "unexpected end of JSON input",
		},
		{
			name:       "Empty input",
			jsonData:   `  `,
			wantErr:    true,
			wantErrMsg: "unexpected end of JSON input",
		},
		{
			name:       "Invalid field type",
			jsonData:   `{"name": "Item1", "value": "one hundred"}`,
			wantErr:    true,
			wantErrMsg: "Failed to unmarshal single item",
		},
		{
			name:        "Empty batch",
			jsonData:    `[]`,
			wantIsBatch: true,
			wantItems:   []MyData{},
			wantErr:     false,
		},
		{
			name:        "Valid empty object",
			jsonData:    `{}`,
			wantIsBatch: false,
			wantItems:   []MyData{{}},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var meta Meta[MyData]
			err := json.Unmarshal([]byte(tt.jsonData), &meta)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Meta.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"Meta.UnmarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}
			if meta.IsBatch != tt.wantIsBatch {
				t.Errorf("Meta.UnmarshalJSON() IsBatch = %v, want %v", meta.IsBatch, tt.wantIsBatch)
			}
			if !reflect.DeepEqual(meta.Items, tt.wantItems) {
				t.Errorf("Meta.UnmarshalJSON() Items = %#v, want %#v", meta.Items, tt.wantItems)
			}
		})
	}
}

func TestMeta_MarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		meta       Meta[MyData]
		wantData   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Single item",
			meta: Meta[MyData]{
				IsBatch: false,
				Items: []MyData{
					{Name: "Item1", Value: 100},
				},
			},
			wantData: `{"name":"Item1","value":100}`,
			wantErr:  false,
		},
		{
			name: "Batch items",
			meta: Meta[MyData]{
				IsBatch: true,
				Items: []MyData{
					{Name: "Item1", Value: 100},
					{Name: "Item2", Value: 200},
				},
			},
			wantData: `[{"name":"Item1","value":100},{"name":"Item2","value":200}]`,
			wantErr:  false,
		},
		{
			name: "Empty items with IsBatch=false",
			meta: Meta[MyData]{
				IsBatch: false,
				Items:   []MyData{},
			},
			wantErr:    true,
			wantErrMsg: "Received empty input",
		},
		{
			name: "Empty items with IsBatch=true",
			meta: Meta[MyData]{
				IsBatch: true,
				Items:   []MyData{},
			},
			wantData: `[]`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(&tt.meta)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Meta.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"Meta.MarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}
			if !jsonStringsEqual(string(data), tt.wantData) {
				t.Errorf("Meta.MarshalJSON() data = %s, want %s", string(data), tt.wantData)
			}
		})
	}
}

// Test MetaRequest UnmarshalJSON and MarshalJSON
func TestMetaRequest_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		wantIsBatch bool
		wantItems   []Request[json.RawMessage]
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "Valid single request",
			jsonData:    `{"jsonrpc":"2.0","method":"sum","params":[1,2,3],"id":1}`,
			wantIsBatch: false,
			wantItems: []Request[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Method:  "sum",
					Params:  json.RawMessage(`[1,2,3]`),
					ID:      &RequestID{Value: 1},
				},
			},
			wantErr: false,
		},
		{
			name:        "Valid batch requests",
			jsonData:    `[{"jsonrpc":"2.0","method":"sum","params":[1,2,3],"id":1},{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":2}]`,
			wantIsBatch: true,
			wantItems: []Request[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Method:  "sum",
					Params:  json.RawMessage(`[1,2,3]`),
					ID:      &RequestID{Value: 1},
				},
				{
					JSONRPC: "2.0",
					Method:  "subtract",
					Params:  json.RawMessage(`[42,23]`),
					ID:      &RequestID{Value: 2},
				},
			},
			wantErr: false,
		},
		{
			name:       "Empty input",
			jsonData:   ``,
			wantErr:    true,
			wantErrMsg: "unexpected end of JSON input",
		},
		{
			name:       "Null input",
			jsonData:   `null`,
			wantErr:    true,
			wantErrMsg: "Received null data",
		},
		{
			name:        "Empty batch",
			jsonData:    `[]`,
			wantIsBatch: true,
			wantItems:   []Request[json.RawMessage]{},
			wantErr:     false,
		},
		{
			name:       "Invalid JSON",
			jsonData:   `{this is not valid JSON}`,
			wantErr:    true,
			wantErrMsg: "invalid character 't' looking for beginning of object key string",
		},
		{
			name:       "Array with null",
			jsonData:   `[null]`,
			wantErr:    true,
			wantErrMsg: "Received null data",
		},
		{
			name:       "Whitespace input",
			jsonData:   "   ",
			wantErr:    true,
			wantErrMsg: "unexpected end of JSON input",
		},
		{
			name:       "Garbage data",
			jsonData:   `garbage data`,
			wantErr:    true,
			wantErrMsg: "invalid character 'g' looking for beginning of value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metaRequest MetaRequest
			metaRequest.Body = &Meta[Request[json.RawMessage]]{}
			err := json.Unmarshal([]byte(tt.jsonData), metaRequest.Body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MetaRequest.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"MetaRequest.UnmarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}
			if metaRequest.Body.IsBatch != tt.wantIsBatch {
				t.Errorf(
					"MetaRequest.UnmarshalJSON() IsBatch = %v, want %v",
					metaRequest.Body.IsBatch,
					tt.wantIsBatch,
				)
			}
			if !compareRequestSlices(metaRequest.Body.Items, tt.wantItems) {
				t.Errorf(
					"MetaRequest.UnmarshalJSON() Items = %+v, want %+v",
					metaRequest.Body.Items,
					tt.wantItems,
				)
			}
		})
	}
}

func TestMetaRequest_MarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		meta       MetaRequest
		wantData   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Single request",
			meta: MetaRequest{
				Body: &Meta[Request[json.RawMessage]]{
					IsBatch: false,
					Items: []Request[json.RawMessage]{
						{
							JSONRPC: "2.0",
							Method:  "subtract",
							Params:  json.RawMessage(`[42,23]`),
							ID:      &RequestID{Value: 1},
						},
					},
				},
			},
			wantData: `{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}`,
			wantErr:  false,
		},
		{
			name: "Batch requests",
			meta: MetaRequest{
				Body: &Meta[Request[json.RawMessage]]{
					IsBatch: true,
					Items: []Request[json.RawMessage]{
						{
							JSONRPC: "2.0",
							Method:  "sum",
							Params:  json.RawMessage(`[1,2,3]`),
							ID:      &RequestID{Value: 1},
						},
						{
							JSONRPC: "2.0",
							Method:  "subtract",
							Params:  json.RawMessage(`[42,23]`),
							ID:      &RequestID{Value: 2},
						},
					},
				},
			},
			wantData: `[{"jsonrpc":"2.0","method":"sum","params":[1,2,3],"id":1},{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":2}]`,
			wantErr:  false,
		},
		{
			name: "Empty items with IsBatch=false",
			meta: MetaRequest{
				Body: &Meta[Request[json.RawMessage]]{
					IsBatch: false,
					Items:   []Request[json.RawMessage]{},
				},
			},
			wantErr:    true,
			wantErrMsg: "Received empty input",
		},
		{
			name: "Empty items with IsBatch=true",
			meta: MetaRequest{
				Body: &Meta[Request[json.RawMessage]]{
					IsBatch: true,
					Items:   []Request[json.RawMessage]{},
				},
			},
			wantData: `[]`,
			wantErr:  false,
		},
		{
			name: "Nil Body",
			meta: MetaRequest{
				Body: nil,
			},
			wantErr:  false,
			wantData: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.meta.Body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MetaRequest.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"MetaRequest.MarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}

			if !jsonStringsEqual(string(data), tt.wantData) {
				t.Errorf("MetaRequest.MarshalJSON() data = %s, want %s", string(data), tt.wantData)
			}
		})
	}
}

// Test MetaResponse UnmarshalJSON and MarshalJSON
func TestMetaResponse_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		wantIsBatch bool
		wantItems   []Response[json.RawMessage]
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:        "Valid single response",
			jsonData:    `{"jsonrpc":"2.0","result":7,"id":1}`,
			wantIsBatch: false,
			wantItems: []Response[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Result:  json.RawMessage(`7`),
					ID:      &RequestID{Value: 1},
				},
			},
			wantErr: false,
		},
		{
			name:        "Valid batch responses",
			jsonData:    `[{"jsonrpc":"2.0","result":7,"id":1},{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":2}]`,
			wantIsBatch: true,
			wantItems: []Response[json.RawMessage]{
				{
					JSONRPC: "2.0",
					Result:  json.RawMessage(`7`),
					ID:      &RequestID{Value: 1},
				},
				{
					JSONRPC: "2.0",
					Error: &JSONRPCError{
						Code:    -32601,
						Message: "Method not found",
					},
					ID: &RequestID{Value: 2},
				},
			},
			wantErr: false,
		},
		{
			name:       "Empty input",
			jsonData:   ``,
			wantErr:    true,
			wantErrMsg: "unexpected end of JSON input",
		},
		{
			name:       "Null input",
			jsonData:   `null`,
			wantErr:    true,
			wantErrMsg: "Received null data",
		},
		{
			name:        "Empty batch",
			jsonData:    `[]`,
			wantIsBatch: true,
			wantItems:   []Response[json.RawMessage]{},
			wantErr:     false,
		},
		{
			name:       "Invalid JSON",
			jsonData:   `{this is not valid JSON}`,
			wantErr:    true,
			wantErrMsg: "invalid character 't' looking for beginning of object key string",
		},
		{
			name:       "Array with null",
			jsonData:   `[null]`,
			wantErr:    true,
			wantErrMsg: "Received null data",
		},
		{
			name:       "Whitespace input",
			jsonData:   "   ",
			wantErr:    true,
			wantErrMsg: "unexpected end of JSON input",
		},
		{
			name:       "Garbage data",
			jsonData:   `garbage data`,
			wantErr:    true,
			wantErrMsg: "invalid character 'g' looking for beginning of value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metaResponse MetaResponse
			metaResponse.Body = &Meta[Response[json.RawMessage]]{}
			err := json.Unmarshal([]byte(tt.jsonData), metaResponse.Body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MetaResponse.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"MetaResponse.UnmarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}
			if metaResponse.Body.IsBatch != tt.wantIsBatch {
				t.Errorf(
					"MetaResponse.UnmarshalJSON() IsBatch = %v, want %v",
					metaResponse.Body.IsBatch,
					tt.wantIsBatch,
				)
			}
			eq, err := jsonStructEqual(metaResponse.Body.Items, tt.wantItems)
			if err != nil || !eq {
				t.Errorf(
					"MetaResponse.UnmarshalJSON() Items = %+v, want %+v",
					metaResponse.Body.Items,
					tt.wantItems,
				)
			}
		})
	}
}

func TestMetaResponse_MarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		meta       MetaResponse
		wantData   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Single response with result",
			meta: MetaResponse{
				Body: &Meta[Response[json.RawMessage]]{
					IsBatch: false,
					Items: []Response[json.RawMessage]{
						{
							JSONRPC: "2.0",
							Result:  json.RawMessage(`7`),
							ID:      &RequestID{Value: 1},
						},
					},
				},
			},
			wantData: `{"jsonrpc":"2.0","result":7,"id":1}`,
			wantErr:  false,
		},
		{
			name: "Single response with error",
			meta: MetaResponse{
				Body: &Meta[Response[json.RawMessage]]{
					IsBatch: false,
					Items: []Response[json.RawMessage]{
						{
							JSONRPC: "2.0",
							Error: &JSONRPCError{
								Code:    -32601,
								Message: "Method not found",
							},
							ID: &RequestID{Value: 2},
						},
					},
				},
			},
			wantData: `{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":2}`,
			wantErr:  false,
		},
		{
			name: "Batch responses",
			meta: MetaResponse{
				Body: &Meta[Response[json.RawMessage]]{
					IsBatch: true,
					Items: []Response[json.RawMessage]{
						{
							JSONRPC: "2.0",
							Result:  json.RawMessage(`7`),
							ID:      &RequestID{Value: 1},
						},
						{
							JSONRPC: "2.0",
							Error: &JSONRPCError{
								Code:    -32601,
								Message: "Method not found",
							},
							ID: &RequestID{Value: 2},
						},
					},
				},
			},
			wantData: `[{"jsonrpc":"2.0","result":7,"id":1},{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":2}]`,
			wantErr:  false,
		},
		{
			name: "Empty items with IsBatch=false",
			meta: MetaResponse{
				Body: &Meta[Response[json.RawMessage]]{
					IsBatch: false,
					Items:   []Response[json.RawMessage]{},
				},
			},
			wantErr:    true,
			wantErrMsg: "Received empty input",
		},
		{
			name: "Empty items with IsBatch=true",
			meta: MetaResponse{
				Body: &Meta[Response[json.RawMessage]]{
					IsBatch: true,
					Items:   []Response[json.RawMessage]{},
				},
			},
			wantData: `[]`,
			wantErr:  false,
		},
		{
			name: "Nil Body",
			meta: MetaResponse{
				Body: nil,
			},
			wantData: "null",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.meta.Body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MetaResponse.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"MetaResponse.MarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}

			if !jsonStringsEqual(string(data), tt.wantData) {
				t.Errorf("MetaResponse.MarshalJSON() data = %s, want %s", string(data), tt.wantData)
			}
		})
	}
}
