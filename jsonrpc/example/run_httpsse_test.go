//go:build integration

package example

import "testing"

// go test -v -tags=integration -run TestRunServer -count=1 ./jsonrpc/example &
func TestRunServer(t *testing.T) {
	// Start the server
	StartHTTPServer()
}
