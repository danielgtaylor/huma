package humafiber_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	HelloRequestBody struct {
		Name string `json:"name"`
	}

	HelloResponseBody struct {
		Message          string `json:"message"`
		FiberUserValue   string `json:"fiber-user-value"`
		FiberUserContext string `json:"fiber-user-context"`
		Huma             string `json:"huma"`
	}

	HelloRequest struct {
		Delay string `query:"huma-fiber-delay"`
		Body  HelloRequestBody
	}

	HelloResponse struct {
		Body HelloResponseBody
	}

	contextKeyFiberUserValue   string
	contextKeyFiberUserContext string
	contextKeyHuma             string
)

const (
	contextValueFiberUserValue   = contextKeyFiberUserValue("context-fiber-user-value")
	contextValueFiberUserContext = contextKeyFiberUserContext("context-fiber-user-context")
	contextValueHuma             = contextKeyHuma("context-huma")
)

var (
	HeaderNameFiberUserValue   = http.CanonicalHeaderKey("fiber-user-value")
	HeaderNameFiberUserContext = http.CanonicalHeaderKey("fiber-user-context")
	HeaderNameHuma             = http.CanonicalHeaderKey("huma")
)

const (
	PingPath  = "/ping"
	HelloPath = "/hello"
)

func PingHandler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusOK)
}

func RegisterPing(app *fiber.App) {
	_ = app.Get(PingPath, PingHandler)
}

func CallPing(ctx context.Context, server string, timeout time.Duration) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server+PingPath, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return err
	}
	if response == nil {
		return errors.New("response is empty")
	}
	if response.StatusCode != fiber.StatusOK {
		return fmt.Errorf("unexpected status code %d", response.StatusCode)
	}
	return nil
}

func WaitPing(ctx context.Context, server string, timeout time.Duration) error {
	for {
		after := time.After(timeout)
		err := CallPing(ctx, server, timeout)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-after:
		}
	}
}

func SimulateAccessToContextOutsideHandler(
	global context.Context,
	wait *sync.WaitGroup,
	timeout time.Duration,
	retries int,
) func(ctx context.Context) {
	return func(ctx context.Context) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			global, cancel := context.WithTimeout(global, timeout*time.Duration(retries))
			defer cancel()
			for {
				_, _ = ctx.Deadline()
				_ = ctx.Done()
				_ = ctx.Err()
				_ = ctx.Value(contextValueFiberUserValue)
				_ = ctx.Value(contextValueFiberUserContext)
				_ = ctx.Value(contextValueHuma)
				select {
				case <-global.Done():
					return
				case <-time.After(timeout / 10):
				}
			}
		}()
	}
}

func HelloHandler(simulator func(context.Context)) func(ctx context.Context, request *HelloRequest) (response *HelloResponse, err error) {
	return func(ctx context.Context, request *HelloRequest) (response *HelloResponse, err error) {
		simulator(ctx)
		var delay time.Duration
		if request.Delay != "" {
			var err error
			if delay, err = time.ParseDuration(request.Delay); err != nil {
				return nil, err
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		var responseBody = HelloResponseBody{
			Message: fmt.Sprintf("Hello, %s!", request.Body.Name),
		}
		if raw := ctx.Value(contextValueFiberUserValue); raw != nil {
			responseBody.FiberUserValue = raw.(string)
		}
		if raw := ctx.Value(contextValueFiberUserContext); raw != nil {
			responseBody.FiberUserContext = raw.(string)
		}
		if raw := ctx.Value(contextValueHuma); raw != nil {
			responseBody.Huma = raw.(string)
		}
		return &HelloResponse{
			Body: responseBody,
		}, nil
	}
}

func HelloOperation() huma.Operation {
	return huma.Operation{
		OperationID:   "Hello",
		Method:        fiber.MethodPost,
		Path:          HelloPath,
		Description:   "Hello description",
		Tags:          []string{"hello"},
		DefaultStatus: fiber.StatusOK,
	}
}

func HelloResponseValidate(t *testing.T, expected HelloResponseBody, response *http.Response) {
	assert.NotNil(t, response)
	assert.Equal(t, fiber.StatusOK, response.StatusCode)
	var actual HelloResponseBody
	err := json.NewDecoder(response.Body).Decode(&actual)
	if assert.NoError(t, err) {
		assert.Equal(t, expected, actual)
	}
}

func FiberMiddlewareUserValue(c *fiber.Ctx) error {
	headers := c.GetReqHeaders()
	if values, found := headers[HeaderNameFiberUserValue]; found && len(values) > 0 {
		c.Context().SetUserValue(contextValueFiberUserValue, values[0])
	}
	return c.Next()
}

func FiberMiddlewareUserContext(c *fiber.Ctx) error {
	headers := c.GetReqHeaders()
	if values, found := headers[HeaderNameFiberUserContext]; found && len(values) > 0 {
		var original = c.UserContext()
		var result = context.WithValue(original, contextValueFiberUserContext, values[0])
		c.SetUserContext(result)
		defer c.SetUserContext(original)
	}
	return c.Next()
}

func HumaMiddleware(ctx huma.Context, next func(huma.Context)) {
	value := ctx.Header(HeaderNameHuma)
	if value != "" {
		ctx = huma.WithValue(ctx, contextValueHuma, value)
	}
	next(ctx)
}

func TestHumaFiber(t *testing.T) {
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

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})
	app.Use(FiberMiddlewareUserValue)
	app.Use(FiberMiddlewareUserContext)
	RegisterPing(app)

	config := huma.DefaultConfig("hello", "1.0.0")
	api := humafiber.New(app, config)
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

	request, err := http.NewRequestWithContext(ctx, fiber.MethodPost, server+HelloPath, requestBodyReader)
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
