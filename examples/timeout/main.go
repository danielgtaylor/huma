package main

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma"
)

func main() {
	r := huma.NewRouter("Timeout Example", "1.0.0")

	r.Resource("/timeout",
		huma.ContextDependency(),
	).Get("timeout example", func(ctx context.Context) string {
		// Add a timeout to the context. No request should take longer than 2 seconds
		newCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		// Create a new request that will take 5 seconds to complete.
		req, _ := http.NewRequestWithContext(
			newCtx, http.MethodGet, "https://httpstat.us/418?sleep=5000", nil)

		// Make the request. This will return with an error because the context
		// deadline of 2 seconds is shorter than the request duration of 5 seconds.
		_, err := http.DefaultClient.Do(req)
		if err != nil {
			return err.Error()
		}

		return "success"
	})

	r.Run()
}
