// Package modbus speaks Modbus TCP directly rather than wrapping a library.
// The original FrostyGoop sample imported github.com/rolfl/modbus; we build the
// frames ourselves so the bytes on the wire can be diffed field by field
// against the sample's captures, which is one of the deliverables.
package modbus

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

const hexDigits = "0123456789abcdef"
const retryBackoff = 50 * time.Millisecond

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
	addr string
	opt  Options

	// Modbus TCP echoes the transaction ID back in the response, so it is the
	// only way to tell a reply from a stale frame left in the socket.
	txID uint16
}

// ModbusError represents a Modbus exception response.
type ModbusError struct {
	FunctionCode  byte
	ExceptionCode byte
}

func (e *ModbusError) Error() string {
	return fmt.Sprintf("modbus exception: function=0x%02x code=0x%02x", e.FunctionCode, e.ExceptionCode)
}

func Dial(addr string, opt Options) (*Conn, error) {
	d := net.Dialer{Timeout: opt.Timeout}
	nc, err := d.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := &Conn{conn: nc, addr: addr, opt: opt}
	if opt.Timeout > 0 {
		nc.SetDeadline(time.Now().Add(opt.Timeout))
	}
	return c, nil
}

func (c *Conn) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Conn) nextTxID() uint16 {
	c.txID++
	return c.txID
}

func (c *Conn) logFrame(dir string, frame []byte) {
	if c.opt.Debug {
		hexStr := ""
		for i, b := range frame {
			if i > 0 {
				hexStr += " "
			}
			hexStr += fmt.Sprintf("%02x", b)
		}
		fmt.Fprintf(os.Stderr, "[%s] %s\n", dir, hexStr)
	}
}

func (c *Conn) sendRecv(ctx context.Context, pdu []byte) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= c.opt.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoff):
			}
			if c.opt.Debug {
				fmt.Fprintf(os.Stderr, "retry %d after: %v\n", attempt, lastErr)
			}
			if err := c.reconnect(); err != nil {
				lastErr = err
				continue
			}
		}

		respPDU, err := c.doSendRecv(pdu)
		if err == nil {
			return respPDU, nil
		}
		lastErr = err

		var mbErr *ModbusError
		if errors.As(err, &mbErr) {
			return nil, err
		}
	}

	return nil, lastErr
}

func (c *Conn) reconnect() error {
	if c.conn != nil {
		c.conn.Close()
	}
	d := net.Dialer{Timeout: c.opt.Timeout}
	nc, err := d.Dial("tcp", c.addr)
	if err != nil {
		return err
	}
	c.conn = nc
	return nil
}

func (c *Conn) doSendRecv(pdu []byte) ([]byte, error) {
	txID := c.nextTxID()

	mbap := make([]byte, 7)
	binary.BigEndian.PutUint16(mbap[0:2], txID)
	binary.BigEndian.PutUint16(mbap[2:4], 0)
	binary.BigEndian.PutUint16(mbap[4:6], uint16(len(pdu)+1))
	mbap[6] = c.opt.UnitID

	frame := append(mbap, pdu...)

	if c.opt.Debug {
		c.logFrame("TX", frame)
	}

	if c.opt.Timeout > 0 {
		c.conn.SetDeadline(time.Now().Add(c.opt.Timeout))
	}
	if _, err := c.conn.Write(frame); err != nil {
		return nil, err
	}

	respHeader := make([]byte, 7)
	if _, err := readFull(c.conn, respHeader); err != nil {
		return nil, err
	}

	respTxID := binary.BigEndian.Uint16(respHeader[0:2])
	if respTxID != txID {
		return nil, errors.New("transaction ID mismatch")
	}
	respProto := binary.BigEndian.Uint16(respHeader[2:4])
	if respProto != 0 {
		return nil, errors.New("invalid protocol ID in response")
	}
	respUnitID := respHeader[6]
	if respUnitID != c.opt.UnitID {
		return nil, errors.New("unit ID mismatch")
	}
	respLen := binary.BigEndian.Uint16(respHeader[4:6])
	if respLen < 2 {
		return nil, errors.New("response too short")
	}

	pduLen := int(respLen) - 1
	respPDU := make([]byte, pduLen)
	if _, err := readFull(c.conn, respPDU); err != nil {
		return nil, err
	}

	if c.opt.Debug {
		c.logFrame("RX", append(respHeader, respPDU...))
	}

	if len(respPDU) >= 2 && respPDU[0]&0x80 != 0 {
		return nil, &ModbusError{
			FunctionCode:  respPDU[0] & 0x7F,
			ExceptionCode: respPDU[1],
		}
	}

	return respPDU, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// hexDump renders a frame as space-separated hex. The output is diffed against
// the sample's Wireshark captures field by field, so it carries no padding and
// no trailing separator.
func hexDump(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	out := make([]byte, 0, len(data)*3-1)
	for i, b := range data {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, hexDigits[b>>4], hexDigits[b&0x0f])
	}
	return string(out)
}
