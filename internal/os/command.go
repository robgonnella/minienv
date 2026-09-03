package os

import (
	"io"
	"os/exec"
)

type CommandRunner interface {
	Run() error
	Output() ([]byte, error)
	Stdin(io.Reader)
	Stdout(io.Writer)
	Stderr(io.Writer)
}

type Commander interface {
	Command(name string, args ...string) CommandRunner
}

type OsCmd struct {
	cmd *exec.Cmd
}

func (c *OsCmd) Run() error {
	return c.cmd.Run()
}

func (c *OsCmd) Output() ([]byte, error) {
	return c.cmd.Output()
}

func (c *OsCmd) Stdin(r io.Reader) {
	c.cmd.Stdin = r
}

func (c *OsCmd) Stdout(w io.Writer) {
	c.cmd.Stdout = w
}

func (c *OsCmd) Stderr(w io.Writer) {
	c.cmd.Stderr = w
}

type OsCommander struct{}

func NewOsCommander() Commander {
	return &OsCommander{}
}

func (c *OsCommander) Command(name string, args ...string) CommandRunner {
	cmd := exec.Command(name, args...)
	return &OsCmd{cmd}
}
