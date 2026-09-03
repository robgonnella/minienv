package config

import "fmt"

type Error struct {
	msg string
}

func Errorf(format string, args ...any) *Error {
	return &Error{msg: fmt.Sprintf(format, args...)}
}

func (c *Error) Error() string {
	return c.msg
}
