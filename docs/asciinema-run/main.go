package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

const CtrlPrefix = "#$"

var (
	Usage          = "usage: " + os.Args[0] + " <script>"
	ErrUnknownCtrl = errors.New("unknown control command")
	ErrNoArgs      = errors.New("no arguments given to command")
	ErrBadArg      = errors.New("invalid command argument")
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		log.Fatal(Usage)
	}
	if exec.Command("asciinema", "-h").Run() != nil {
		log.Fatal("can't find asciinema executable")
	}

	s, err := NewScript(os.Args[1], os.Args[2:])
	if err != nil {
		log.Fatal("parsing script failed: ", err)
	}

	if err := s.Start(); err != nil {
		log.Fatal("couldn't start recording: ", err)
	}
	defer func() {
		if err := s.Stop(); err != nil {
			log.Fatal("couldn't stop recording: ", err)
		}
	}()

	s.Execute()
}

// Command is an action to be run.
type Command interface {
	Run(*Script)
}

// Shell is a shell command to execute.
type Shell struct {
	Cmd string
}

// NewShell creates a new Shell.
func NewShell(cmd string) Shell {
	if !strings.HasSuffix(cmd, "\n") {
		cmd += "\n"
	}
	return Shell{Cmd: cmd}
}

// Run runs the shell command.
func (s Shell) Run(sc *Script) {
	for _, c := range s.Cmd {
		if _, err := sc.Stdin.Write([]byte(string(c))); err != nil {
			os.Exit(1)
		}
		time.Sleep(sc.Delay)
	}
}

// Wait is a command to change the interval between commands.
type Wait struct {
	Duration time.Duration
}

// NewWait creates a new Wait.
func NewWait(opts []string) (Wait, error) {
	if len(opts) == 0 {
		return Wait{}, ErrNoArgs
	}

	ms, err := strconv.ParseInt(strings.TrimSpace(opts[0]), 10, 64)
	if err != nil {
		return Wait{}, ErrBadArg
	}

	return Wait{Duration: time.Millisecond * time.Duration(ms)}, nil
}

// Run changes the wait for subsequent commands.
func (w Wait) Run(s *Script) {
	s.Wait = w.Duration
}

// Delay is a command to change the typing speed of subsequent commands.
type Delay struct {
	Interval time.Duration
}

// NewDelay creates a new Delay.
func NewDelay(opts []string) (Delay, error) {
	if len(opts) == 0 {
		return Delay{}, ErrNoArgs
	}

	ms, err := strconv.ParseInt(strings.TrimSpace(opts[0]), 10, 64)
	if err != nil {
		return Delay{}, ErrBadArg
	}

	return Delay{Interval: time.Millisecond * time.Duration(ms)}, nil
}

// Run changes the typing speed for subsequent commands.
func (s Delay) Run(sc *Script) {
	sc.Delay = s.Interval
}

// NewCtrl creates a new control command.
func NewCtrl(cmd string) (Command, error) {
	tokens := strings.Split(cmd, " ")
	switch strings.TrimSpace(tokens[0]) {
	case "delay":
		return NewDelay(tokens[1:])
	case "wait":
		return NewWait(tokens[1:])
	default:
		return nil, ErrUnknownCtrl
	}
}

// Script is a shell script to be run and recorded by asciinema.
type Script struct {
	Args     []string
	Commands []Command
	Delay    time.Duration
	Wait     time.Duration
	Cmd      *exec.Cmd
	Stdin    io.WriteCloser
	Stdout   io.ReadCloser
	Stderr   io.ReadCloser
}

// NewScript parses a new Script from the script file at path.
func NewScript(path string, args []string) (*Script, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	s := &Script{
		Args:  args,
		Delay: time.Millisecond * 40,
		Wait:  time.Millisecond * 100,
	}

	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		if line == "" && i == len(lines)-1 {
			continue
		}
		if i == len(lines)-1 && line == "" {
			continue
		}
		if strings.HasPrefix(line, CtrlPrefix) {
			ctrl, err := NewCtrl(strings.TrimSpace(line[len(CtrlPrefix):]))
			if err != nil {
				return nil, fmt.Errorf("%v (line %d)", err, i+1)
			}
			s.Commands = append(s.Commands, ctrl)
		} else {
			s.Commands = append(s.Commands, NewShell(line))
		}
	}

	return s, nil
}

// Start starts recording.
func (s *Script) Start() error {
	args := append([]string{"rec"}, s.Args...)
	s.Cmd = exec.Command("asciinema", args...)
	var err error
	if s.Stdin, err = s.Cmd.StdinPipe(); err != nil {
		return err
	}
	if s.Stdout, err = s.Cmd.StdoutPipe(); err != nil {
		return err
	}
	if s.Stderr, err = s.Cmd.StderrPipe(); err != nil {
		return err
	}
	if err = s.Cmd.Start(); err != nil {
		return err
	}
	go echo(s.Stdout)
	go echo(s.Stderr)
	return nil
}

// Stop stops recording.
func (s *Script) Stop() error {
	defer s.Cmd.Wait()
	if _, err := s.Stdin.Write([]byte{4}); err != nil {
		return err
	}
	if len(s.Args) == 0 || strings.HasPrefix(s.Args[0], "-") {
		s.endDialog()
	}
	return nil
}

func (s *Script) endDialog() {
	handler := make(chan os.Signal, 1)
	sig := make(chan os.Signal, 1)
	stdin := make(chan bool, 1)
	signal.Notify(handler, os.Interrupt)

	go func() {
		sig <- <-handler
		signal.Stop(handler)

	}()
	go func() {
		fmt.Scanln()
		stdin <- true
	}()

	select {
	case int := <-sig:
		s.Cmd.Process.Signal(int)
	case <-stdin:
		s.Stdin.Write([]byte{'\n'})
	}
}

// Execute runs the script's commands.
func (s *Script) Execute() {
	for _, c := range s.Commands {
		c.Run(s)
		time.Sleep(s.Wait)
	}
}

// echo prints out output continously.
func echo(r io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if err != nil {
			return
		}
		fmt.Print(string(buf[:n]))
	}
}
