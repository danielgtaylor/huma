package huma

import (
	"context"
	"testing"
	"time"
)

func TestServerShutdown(t *testing.T) {
	r := NewTestRouter(t)

	go func() {
		r.Root().Run(nil, []string{})
	}()

	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	r.Shutdown(ctx)
}
