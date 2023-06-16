package huma

import (
	"bytes"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCLIPlain(t *testing.T) {
	type Options struct {
		Debug bool
		Host  string
		Port  int

		// ignore private fields, should not crash.
		ingore bool
	}

	cli := NewCLI(func(cli CLI, options *Options) {
		assert.Equal(t, true, options.Debug)
		assert.Equal(t, "localhost", options.Host)
		assert.Equal(t, 8001, options.Port)
		assert.Equal(t, false, options.ingore)
		cli.OnStart(func() {
			// Do nothing
		})
	})

	cli.Root().SetArgs([]string{"--debug", "--host", "localhost", "--port", "8001"})
	cli.Run()
}

func TestCLIAdvanced(t *testing.T) {
	type DebugOption struct {
		Debug bool `doc:"Enable debug mode." default:"false"`
	}

	type Options struct {
		// Example of option composition via embedded type.
		DebugOption
		Host string `doc:"Hostname to listen on."`
		Port int    `doc:"Port to listen on." short:"p" default:"8000"`
	}

	cli := NewCLI(func(cli CLI, options *Options) {
		assert.Equal(t, true, options.Debug)
		assert.Equal(t, "localhost", options.Host)
		assert.Equal(t, 8001, options.Port)
		cli.OnStart(func() {
			// Do nothing
		})
	})

	// A custom pre-run isn't overwritten and should still work!
	customPreRun := false
	cli.Root().PersistentPreRun = func(cmd *cobra.Command, args []string) {
		customPreRun = true
	}

	cli.Root().SetArgs([]string{"--debug", "--host", "localhost", "--port", "8001"})
	cli.Run()
	assert.True(t, customPreRun)
}

func TestCLIHelp(t *testing.T) {
	type Options struct {
		Debug bool
		Host  string
		Port  int
	}

	cli := NewCLI(func(cli CLI, options *Options) {
		// Do nothing
	})

	cli.Root().SetArgs([]string{"--help"})
	buf := bytes.NewBuffer(nil)
	cli.Root().SetOut(buf)
	cli.Root().SetErr(buf)
	cli.Run()

	assert.Equal(t, "Usage:\n  myapp [flags]\n\nFlags:\n      --debug         \n  -h, --help          help for myapp\n      --host string   \n      --port int\n", buf.String())
}

func TestCLICommandWithOptions(t *testing.T) {
	type Options struct {
		Debug bool
	}

	cli := NewCLI(func(cli CLI, options *Options) {
		// Do nothing
	})

	wasSet := false
	cli.Root().AddCommand(&cobra.Command{
		Use: "custom",
		Run: WithOptions(func(cmd *cobra.Command, args []string, options *Options) {
			if options.Debug {
				wasSet = true
			}
		}),
	})

	cli.Root().SetArgs([]string{"custom", "--debug"})
	cli.Run()

	assert.True(t, wasSet)
}

func TestCLIShutdown(t *testing.T) {
	type Options struct{}

	started := false
	stopping := make(chan bool, 1)
	cli := NewCLI(func(cli CLI, options *Options) {
		cli.OnStart(func() {
			started = true
			<-stopping
		})
		cli.OnStop(func() {
			stopping <- true
		})
	})

	go func() {
		time.Sleep(10 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	cli.Root().SetArgs([]string{})
	cli.Run()
	assert.True(t, started)
}

func TestCLIBadType(t *testing.T) {
	type Options struct {
		Debug []struct{}
	}

	assert.Panics(t, func() {
		NewCLI(func(cli CLI, options *Options) {})
	})
}

func TestCLIBadDefaults(t *testing.T) {
	type OptionsBool struct {
		Debug bool `default:"notabool"`
	}

	type OptionsInt struct {
		Debug int `default:"notanint"`
	}

	assert.Panics(t, func() {
		NewCLI(func(cli CLI, options *OptionsBool) {})
	})

	assert.Panics(t, func() {
		NewCLI(func(cli CLI, options *OptionsInt) {})
	})
}
