package huma

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCLIPlain(t *testing.T) {
	type Options struct {
		Debug bool
		Host  string
		Port  int
	}

	cli := NewCLI(func(cli CLI, options *Options) {
		assert.Equal(t, true, options.Debug)
		assert.Equal(t, "localhost", options.Host)
		assert.Equal(t, 8001, options.Port)
		cli.OnStart(func() {
			// Do nothing
		})
	})

	cli.Root().SetArgs([]string{"--debug", "--host", "localhost", "--port", "8001"})
	cli.Run()
}

func TestCLITags(t *testing.T) {
	type Options struct {
		Debug bool   `doc:"Enable debug mode." default:"false"`
		Host  string `doc:"Hostname to listen on."`
		Port  int    `doc:"Port to listen on." short:"p" default:"8000"`
	}

	cli := NewCLI(func(cli CLI, options *Options) {
		assert.Equal(t, true, options.Debug)
		assert.Equal(t, "localhost", options.Host)
		assert.Equal(t, 8001, options.Port)
		cli.OnStart(func() {
			// Do nothing
		})
	})

	cli.Root().SetArgs([]string{"--debug", "--host", "localhost", "--port", "8001"})
	cli.Run()
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
