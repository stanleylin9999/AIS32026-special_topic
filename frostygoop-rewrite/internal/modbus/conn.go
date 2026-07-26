// Package modbus speaks Modbus TCP directly rather than wrapping a library.
// The original FrostyGoop sample imported github.com/rolfl/modbus; we build the
// frames ourselves so the bytes on the wire can be diffed field by field
// against the sample's captures, which is one of the deliverables.
package modbus

import (
	"errors"
	"net"
	"time"
)

var errNotImplemented = errors.New("not implemented")

// Options carries the sample's CLI knobs down into the transport. Retry and
// timeout live here so callers never reimplement them per call site.
type Options struct {
	Timeout time.Duration
	Retries int
	UnitID  byte
	Debug   bool
}

type Conn struct {
	conn net.Conn
	opt  Options

	// Modbus TCP echoes the transaction ID back in the response, so it is the
	// only way to tell a reply from a stale frame left in the socket.
	txID uint16
}

func Dial(addr string, opt Options) (*Conn, error) {
	return nil, errNotImplemented
}

func (c *Conn) Close() error {
	return errNotImplemented
}
