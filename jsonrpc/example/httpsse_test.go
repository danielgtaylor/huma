package example

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/jsonrpc"
)

type JSONRPCClient interface {
	Send(reqBytes []byte) ([]byte, error)
}

type HTTPJSONRPCClient struct {
	client *http.Client
	url    string
}

func NewHTTPClient(t *testing.T) *HTTPJSONRPCClient {
	handler := SetupSSETransport()
	server := httptest.NewUnstartedServer(handler)
	server.Start()
	t.Cleanup(server.Close) // Ensure server closes after test
	client := server.Client()
	url := server.URL + "/jsonrpc"
	return &HTTPJSONRPCClient{
		client: client,
		url:    url,
	}
}

func (c *HTTPJSONRPCClient) Send(reqBytes []byte) ([]byte, error) {
	resp, err := c.client.Post(c.url, "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func getClient(t *testing.T) JSONRPCClient {
	return NewHTTPClient(t)
}

func sendJSONRPCRequest(t *testing.T, client JSONRPCClient, request interface{}) []byte {
	var reqBytes []byte
	var err error
	if b, ok := request.([]byte); ok {
		reqBytes = b
	} else {
		reqBytes, err = json.Marshal(request)
		if err != nil {
			t.Fatalf("Error marshaling request: %v", err)
		}
	}
	t.Logf("Sending req %s", string(reqBytes))
	respBody, err := client.Send(reqBytes)
	if err != nil {
		t.Fatalf("Error sending request: %v", err)
	}
	if len(respBody) == 0 {
		t.Log("Got Empty response")
		return nil
	}
	var o interface{}
	err = json.Unmarshal(respBody, &o)
	if err == nil {
		r, err := json.Marshal(o)
		if err == nil {
			t.Logf("Json resp %s", string(r))
		}
	}
	return respBody
}

func TestValidSingleRequests(t *testing.T) {
	client := getClient(t)

	tests := []struct {
		name           string
		request        interface{}
		expectedResult interface{}
	}{
		{
			name: "Add method with named parameters",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "add",
				"params":  map[string]interface{}{"a": 2, "b": 3},
				"id":      1,
			},
			expectedResult: map[string]float64{"sum": 5},
		},
		{
			name: "Add method with positional parameters",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "addpositional",
				"params":  []interface{}{2, 3},
				"id":      2,
			},
			expectedResult: map[string]float64{"sum": 5},
		},
		{
			name: "Echo method with no parameters",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "echo",
				"id":      3,
			},
			expectedResult: nil,
		},
		{
			name: "Echo method with optional parameters",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "echooptional",
				"id":      "1",
				"params":  "foo",
			},
			expectedResult: "foo",
		},
		{
			name: "Echo method with optional parameters nil input",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "echooptional",
				"id":      "2",
			},
			expectedResult: nil,
		},
		{
			name: "Concat method",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "concat",
				"params":  map[string]interface{}{"s1": "Hello, ", "s2": "World!"},
				"id":      2,
			},
			expectedResult: "Hello, World!",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			respBody := sendJSONRPCRequest(t, client, tc.request)

			var response struct {
				JSONRPC      string                `json:"jsonrpc"`
				Result       interface{}           `json:"result"`
				JSONRPCError *jsonrpc.JSONRPCError `json:"error"`
				ID           interface{}           `json:"id"`
			}

			err := json.Unmarshal(respBody, &response)
			if err != nil {
				t.Fatalf("Error unmarshaling response: %v", err)
			}

			if response.JSONRPCError != nil {
				t.Errorf("Expected no error, but got: %+v", response.JSONRPCError)
			} else {
				eq, err := jsonStructEqual(response.Result, tc.expectedResult)
				if err != nil || !eq {
					t.Errorf("Expected result %#v, got %#v", tc.expectedResult, response.Result)
				}
			}
		})
	}
}

func TestInvalidSingleRequests(t *testing.T) {
	client := getClient(t)

	tests := []struct {
		name          string
		request       interface{}
		rawRequest    []byte
		expectedError *jsonrpc.JSONRPCError
	}{
		{
			name:       "Invalid JSON request",
			rawRequest: []byte(`{ this is invalid json }`),
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.ParseError,
				Message: jsonrpc.GetDefaultErrorMessage(jsonrpc.ParseError),
			},
		},
		{
			name: "Method not found",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "unknown_method",
				"id":      1,
			},
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.MethodNotFoundError,
				Message: "Method 'unknown_method' not found",
			},
		},
		{
			name: "Invalid parameters",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "add",
				"params":  map[string]interface{}{"a": "two", "b": 3},
				"id":      2,
			},
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.InvalidRequestError,
				Message: jsonrpc.GetDefaultErrorMessage(jsonrpc.InvalidRequestError),
			},
		},
		{
			name: "Missing jsonrpc field",
			request: map[string]interface{}{
				"method": "add",
				"params": map[string]interface{}{"a": 2, "b": 3},
				"id":     3,
			},
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.InvalidRequestError,
				Message: jsonrpc.GetDefaultErrorMessage(jsonrpc.InvalidRequestError),
			},
		},
		{
			name: "Invalid jsonrpc version",
			request: map[string]interface{}{
				"jsonrpc": "1.0",
				"method":  "add",
				"params":  map[string]interface{}{"a": 2, "b": 3},
				"id":      4,
			},
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.InvalidRequestError,
				Message: jsonrpc.GetDefaultErrorMessage(jsonrpc.InvalidRequestError),
			},
		},
		{
			name: "Missing method field",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"params":  map[string]interface{}{"a": 2, "b": 3},
				"id":      5,
			},
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.InvalidRequestError,
				Message: jsonrpc.GetDefaultErrorMessage(jsonrpc.InvalidRequestError),
			},
		},
		{
			name: "Invalid id field (array)",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "add",
				"params":  map[string]interface{}{"a": 2, "b": 3},
				"id":      []int{1, 2, 3},
			},
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.InvalidRequestError,
				Message: jsonrpc.GetDefaultErrorMessage(jsonrpc.InvalidRequestError),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.request
			if tc.rawRequest != nil {
				req = tc.rawRequest
			}

			respBody := sendJSONRPCRequest(t, client, req)

			var response struct {
				JSONRPC      string                `json:"jsonrpc"`
				Result       interface{}           `json:"result"`
				JSONRPCError *jsonrpc.JSONRPCError `json:"error"`
				ID           interface{}           `json:"id"`
			}

			err := json.Unmarshal(respBody, &response)
			if err != nil {
				t.Fatalf("Error unmarshaling response: %v", err)
			}

			if response.JSONRPCError == nil {
				t.Errorf("Expected error but got none")
			} else {
				if response.JSONRPCError.Code != tc.expectedError.Code {
					t.Errorf("Expected error code %d, got %d", tc.expectedError.Code, response.JSONRPCError.Code)
				}
				if !strings.Contains(response.JSONRPCError.Message, tc.expectedError.Message) {
					t.Errorf("Expected error message '%s', got '%s'", tc.expectedError.Message, response.JSONRPCError.Message)
				}
			}
		})
	}
}

func TestNotifications(t *testing.T) {
	client := getClient(t)

	tests := []struct {
		name          string
		request       interface{}
		expectedError *jsonrpc.JSONRPCError
	}{
		{
			name: "Valid notification",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "notify",
				"params":  map[string]interface{}{"message": "Hello"},
			},
		},
		{
			name: "Notification with invalid method",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "unknown_method",
				"params":  map[string]interface{}{"message": "Hello"},
			},
			expectedError: &jsonrpc.JSONRPCError{
				Code:    jsonrpc.MethodNotFoundError,
				Message: "Method 'unknown_method' not found",
			},
		},
		{
			name: "Ping notification",
			request: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "ping",
				"params":  map[string]interface{}{"message": "Test Ping"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			respBody := sendJSONRPCRequest(t, client, tc.request)

			if len(respBody) != 0 {
				if tc.expectedError != nil {
					var response struct {
						JSONRPC      string                `json:"jsonrpc"`
						Result       interface{}           `json:"result"`
						JSONRPCError *jsonrpc.JSONRPCError `json:"error"`
						ID           interface{}           `json:"id"`
					}
					err := json.Unmarshal(respBody, &response)
					if err != nil {
						t.Fatalf("Error unmarshaling response: %v", err)
					}
					if response.JSONRPCError.Code != tc.expectedError.Code {
						t.Errorf(
							"Expected error code %d, got %d",
							tc.expectedError.Code,
							response.JSONRPCError.Code,
						)
					}
					if !strings.Contains(response.JSONRPCError.Message, tc.expectedError.Message) {
						t.Errorf(
							"Expected error message '%s', got '%s'",
							tc.expectedError.Message,
							response.JSONRPCError.Message,
						)
					}
				} else {
					t.Errorf("Expected no response, but got: %s", string(respBody))
				}
			}

		})
	}
}

func TestBatchRequests(t *testing.T) {
	client := getClient(t)

	tests := []struct {
		name               string
		batchRequest       []interface{}
		expectedResponses  int
		expectedErrorCodes []int
		expectedResults    map[interface{}]interface{}
	}{
		{
			name: "Valid batch with multiple requests",
			batchRequest: []interface{}{
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "add",
					"params":  map[string]interface{}{"a": 1, "b": 2},
					"id":      1,
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "echooptional",
					"params":  "foo",
					"id":      2,
				},
			},
			expectedResponses:  2,
			expectedErrorCodes: []int{},
		},
		{
			name: "Batch with mixed valid requests and notifications",
			batchRequest: []interface{}{
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "add",
					"params":  map[string]interface{}{"a": 1, "b": 2},
					"id":      1,
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "notify",
					"params":  map[string]interface{}{"message": "Hello"},
				},
			},
			expectedResponses:  1,
			expectedErrorCodes: []int{},
		},
		// // This wont work as framer will not allow sending a request for stdio transport
		// {
		// 	name: "Batch with invalid JSON in one request",
		// 	batchRequest: []interface{}{[]byte(`[{
		// 					"jsonrpc": "2.0",
		// 					"method": "add",
		// 					"params": {"a":1,"b":2},
		// 					"id":1
		// 			}, {
		// 					"jsonrpc": "2.0",
		// 					"method": "invalid_method",
		// 					"params": {},
		// 					"id":2
		// 			}`)}, // Incomplete closing square bracket
		// 	expectedResponses:  1,
		// 	expectedErrorCodes: []int{-32700},
		// },
		{
			name: "Batch of notifications",
			batchRequest: []interface{}{
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "notify",
					"params":  map[string]interface{}{"message": "Hello"},
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "notify",
					"params":  map[string]interface{}{"message": "World"},
				},
			},
			expectedResponses:  0,
			expectedErrorCodes: []int{},
		},
		{
			name:               "Empty batch array",
			batchRequest:       []interface{}{},
			expectedResponses:  1,
			expectedErrorCodes: []int{-32600},
		},
		{
			name: "Batch with valid and invalid methods",
			batchRequest: []interface{}{
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "add",
					"params":  map[string]interface{}{"a": 1, "b": 2},
					"id":      1,
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "concat",
					"params":  map[string]interface{}{"s1": "foo", "s2": "bar"},
					"id":      2,
				},
				map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "unknownMethod",
					"id":      3,
				},
			},
			expectedResponses:  1,
			expectedErrorCodes: []int{int(jsonrpc.InvalidRequestError)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var batch interface{}
			if len(tc.batchRequest) > 0 {
				if b, ok := tc.batchRequest[0].([]byte); ok {
					batch = b
				} else {
					batch = tc.batchRequest
				}
			}
			respBody := sendJSONRPCRequest(t, client, batch)

			if tc.expectedResponses == 0 {
				if len(respBody) != 0 {
					t.Errorf("Expected no response, but got: %s", string(respBody))
					return
				} else {
					return
				}
			}

			var responses []struct {
				JSONRPC      string                `json:"jsonrpc"`
				Result       interface{}           `json:"result"`
				JSONRPCError *jsonrpc.JSONRPCError `json:"error"`
				ID           interface{}           `json:"id"`
			}

			if err := json.Unmarshal(respBody, &responses); err != nil {
				var singleResponse struct {
					JSONRPC      string                `json:"jsonrpc"`
					Result       interface{}           `json:"result"`
					JSONRPCError *jsonrpc.JSONRPCError `json:"error"`
					ID           interface{}           `json:"id"`
				}
				if err := json.Unmarshal(respBody, &singleResponse); err != nil {
					t.Fatalf("Error unmarshaling response: %v", err)
				}
				responses = []struct {
					JSONRPC      string                `json:"jsonrpc"`
					Result       interface{}           `json:"result"`
					JSONRPCError *jsonrpc.JSONRPCError `json:"error"`
					ID           interface{}           `json:"id"`
				}{singleResponse}
			}

			if len(responses) != tc.expectedResponses {
				t.Errorf("Expected %d responses, got %d", tc.expectedResponses, len(responses))
			}

			var gotErrorCodes []int
			for _, response := range responses {
				if response.JSONRPCError != nil {
					gotErrorCodes = append(gotErrorCodes, int(response.JSONRPCError.Code))
				}
			}
			if !arraysAreSimilar(gotErrorCodes, tc.expectedErrorCodes) {
				t.Errorf(
					"Mismatched error codes. Got: %#v, Expected: %#v",
					gotErrorCodes,
					tc.expectedErrorCodes,
				)
			}

			if tc.expectedResults != nil {
				for _, response := range responses {
					id := response.ID
					expectedResult, ok := tc.expectedResults[id]
					if ok {
						if response.JSONRPCError != nil {
							t.Errorf(
								"Expected result for id %v, but got error: %+v",
								id,
								response.JSONRPCError,
							)
						} else {
							eq, err := jsonStructEqual(response.Result, expectedResult)
							if err != nil {
								t.Errorf("Error comparing result for id %v: %v", id, err)
							} else if !eq {
								t.Errorf("Mismatched result for id %v. Got: %+v, Expected: %+v", id, response.Result, expectedResult)
							}
						}
					}
				}
			}
		})
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var o1 interface{}
	var o2 interface{}

	if err := json.Unmarshal(a, &o1); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &o2); err != nil {
		return false
	}
	// Direct reflect Deepequal would have issues when there are pointers, keyorders etc.
	// unmarshalling into a interface and then doing deepequal removes those issues
	return reflect.DeepEqual(o1, o2)
}

func jsonStringsEqual(a, b string) bool {
	return jsonEqual([]byte(a), []byte(b))
}

func getJSONStrings(args ...interface{}) ([]string, error) {
	var ret []string
	for _, a := range args {
		jsonBytes, err := json.Marshal(a)
		if err != nil {
			return nil, err
		}
		ret = append(ret, string(jsonBytes))
	}
	return ret, nil
}

func jsonStructEqual(arg1 interface{}, arg2 interface{}) (bool, error) {
	vals, err := getJSONStrings(arg1, arg2)
	if err != nil {
		log.Fatalf("Could not encode struct to json")
	}
	return jsonStringsEqual(vals[0], vals[1]), nil
}

func arraysAreSimilar(arr1, arr2 []int) bool {
	if len(arr1) != len(arr2) {
		return false
	}
	if len(arr1) != 0 {
		counts1 := make(map[int]int)
		counts2 := make(map[int]int)

		for _, num := range arr1 {
			counts1[num]++
		}

		for _, num := range arr2 {
			counts2[num]++
		}

		for key, count1 := range counts1 {
			if count2, exists := counts2[key]; !exists || count1 != count2 {
				return false
			}
		}
	}

	return true
}
