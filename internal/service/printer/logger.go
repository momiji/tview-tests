package printer

import (
	"fmt"
	"time"
)

const timeFormat = "2006/01/02 15:04:05.999"

// Printf formats a line with a leading timestamp and queues it via
// Println, same as the other log-level helpers below.
func (p *Printer) Printf(format string, a ...any) {
	format = fmt.Sprintf("%s %s", time.Now().Format(timeFormat), format)
	p.Println(fmt.Sprintf(format, a...))
}

// Infof logs at info level.
func (p *Printer) Infof(format string, args ...interface{}) {
	p.Printf(format, args...)
}

// Errorf logs at error level.
func (p *Printer) Errorf(format string, args ...interface{}) {
	p.Printf(format, args...)
}
