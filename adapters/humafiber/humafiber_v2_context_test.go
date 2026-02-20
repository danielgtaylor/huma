package humafiber_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	fiberV2 "github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func PingHandlerV2(c *fiberV2.Ctx) error {
	return c.SendStatus(fiberV2.StatusOK)
}

func RegisterPingV2(app *fiberV2.App) {
	_ = app.Get(PingPath, PingHandlerV2)
}

func FiberMiddlewareUserValueV2(c *fiberV2.Ctx) error {
	headers := c.GetReqHeaders()
	if values, found := headers[HeaderNameFiberUserValue]; found && len(values) > 0 {
		c.Context().SetUserValue(contextValueFiberUserValue, values[0])
	}
	return c.Next()
}

func FiberMiddlewareUserContextV2(c *fiberV2.Ctx) error {
	headers := c.GetReqHeaders()
	if values, found := headers[HeaderNameFiberUserContext]; found && len(values) > 0 {
		var original = c.UserContext()
		var result = context.WithValue(original, contextValueFiberUserContext, values[0])
		c.SetUserContext(result)
		defer c.SetUserContext(original)
	}
	return c.Next()
}

func TestHumaFiberV2(t *testing.T) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wait sync.WaitGroup
	defer wait.Wait()

	timeout := time.Millisecond * 10
	retries := 10
	simulator := SimulateAccessToContextOutsideHandler(ctx, &wait, timeout, retries)

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NotZero(t, port)
	server := fmt.Sprintf("http://localhost:%d", port)

	app := fiberV2.New(fiberV2.Config{
		DisableStartupMessage: true,
	})
	app.Use(FiberMiddlewareUserValueV2)
	app.Use(FiberMiddlewareUserContextV2)
	RegisterPingV2(app)

	config := huma.DefaultConfig("hello", "1.0.0")
	api := humafiber.NewV2(app, config)
	api.UseMiddleware(HumaMiddleware)
	huma.Register(api, HelloOperation(), HelloHandler(simulator))

	wait.Add(1)
	go func() {
		defer wait.Done()
		err := app.Listener(ln)
		assert.NoError(t, err)
	}()
	defer wait.Wait()

	err = WaitPing(ctx, server, timeout)
	require.NoError(t, err)

	name := "Bob"
	message := fmt.Sprintf("Hello, %s!", name)
	requestBody, err := json.Marshal(HelloRequestBody{
		Name: name,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, requestBody)
	requestBodyReader := bytes.NewReader(requestBody)
	expected := HelloResponseBody{
		Message:          message,
		FiberUserValue:   "one",
		FiberUserContext: "two",
		Huma:             "three",
	}

	request, err := http.NewRequestWithContext(ctx, fiberV2.MethodPost, server+HelloPath, requestBodyReader)
	require.NoError(t, err)
	request.Header.Add(HeaderNameFiberUserValue, "one")
	request.Header.Add(HeaderNameFiberUserContext, "two")
	request.Header.Add(HeaderNameHuma, "three")
	query := request.URL.Query()
	query.Add("huma-fiber-delay", timeout.String())
	request.URL.RawQuery = query.Encode()

	// simple check
	response, err := http.DefaultClient.Do(request)
	if response != nil && response.Body != nil {
		defer func() {
			_ = response.Body.Close()
		}()
	}
	require.NoError(t, err)
	HelloResponseValidate(t, expected, response)

	// check that delay works
	doneFirst := make(chan bool)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer close(doneFirst)
		response, err := http.DefaultClient.Do(request)
		if response != nil && response.Body != nil {
			defer func() {
				_ = response.Body.Close()
			}()
		}
		assert.NoError(t, err)
		HelloResponseValidate(t, expected, response)
	}()
	select {
	case <-ctx.Done():
		return
	case <-doneFirst:
		assert.Fail(t, "expected other branch")
	default:
		// ok
	}
	select {
	case <-ctx.Done():
		return
	case <-doneFirst:
		// ok
	case <-time.After(timeout * 2):
		assert.Fail(t, "expected other branch")
	}

	// check graceful shutdown
	doneSecond := make(chan bool)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer close(doneSecond)
		response, err := http.DefaultClient.Do(request)
		if response != nil && response.Body != nil {
			defer func() {
				_ = response.Body.Close()
			}()
		}
		assert.NoError(t, err)
		HelloResponseValidate(t, expected, response)
	}()

	// perform shutdown
	doneShutdown := make(chan bool)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer close(doneShutdown)
		time.Sleep(timeout) // delay before shutdown to start request processing
		err := app.ShutdownWithContext(ctx)
		assert.NoError(t, err)
		time.Sleep(timeout) // delay after shutdown to catch request processing
	}()

	// request should be handled
	select {
	case <-ctx.Done():
		return
	case <-doneSecond:
		// ok
	case <-doneShutdown:
		assert.Fail(t, "expected other branch")
	}

	// shutdown should be handled
	select {
	case <-ctx.Done():
		return
	case <-doneShutdown:
		// ok
	}

	wait.Wait()
}
