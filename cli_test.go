package huma

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestServerShutdown(t *testing.T) {
	r := NewTestRouter(t)

	go func() {
		// Let the OS pick a random port.
		os.Setenv("SERVICE_PORT", "0")
		r.Root().Run(nil, []string{})
	}()

	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	r.Shutdown(ctx)
}
