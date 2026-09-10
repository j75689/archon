package log

import (
	"fmt"
	"io"
)

type Logger interface {
	Info(msg string)
	Debug(msg string)
}

type Nop struct{}

func (Nop) Info(string)  {}
func (Nop) Debug(string) {}

type Writer struct {
	W     io.Writer
	Level int
}

func (w Writer) Info(msg string) {
	if w.W == nil || w.Level < 1 {
		return
	}
	fmt.Fprintln(w.W, msg)
}

func (w Writer) Debug(msg string) {
	if w.W == nil || w.Level < 2 {
		return
	}
	fmt.Fprintln(w.W, msg)
}

func OrNop(l Logger) Logger {
	if l == nil {
		return Nop{}
	}
	return l
}

func FromVerbose(w io.Writer, n int) Logger {
	if n <= 0 {
		return Nop{}
	}
	if n > 2 {
		n = 2
	}
	return Writer{W: w, Level: n}
}
