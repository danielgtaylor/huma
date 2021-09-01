package cli

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

func TestParsedArgs(t *testing.T) {
	app := NewRouter("Test API", "1.0.0")

	foo := ""
	app.Flag("foo", "f", "desc", "")

	wg := sync.WaitGroup{}
	wg.Add(1)

	app.Root().AddCommand(&cobra.Command{
		Use: "foo-test",
		Run: func(cmd *cobra.Command, args []string) {
			// Command does nothing...
		},
	})

	app.ArgsParsed(func() {
		foo = viper.GetString("foo")
		wg.Done()
	})

	app.Root().SetArgs([]string{"foo-test", "--foo=bar"})

	go func() {
		app.Root().Execute()
	}()

	wg.Wait()

	assert.Equal(t, "bar", foo)
}
