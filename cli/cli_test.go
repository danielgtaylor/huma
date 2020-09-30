package cli

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCLI(t *testing.T) {
	app := NewRouter("Test API", "1.0.0")

	started := false
	app.PreStart(func() {
		started = true
	})

	go func() {
		// Let the OS pick a random port.
		os.Setenv("SERVICE_PORT", "0")
		os.Setenv("SERVICE_HOST", "127.0.0.1")
		app.Root().Run(nil, []string{})
	}()

	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	app.Shutdown(ctx)

	assert.Equal(t, true, started)
}
