// Package modbus speaks Modbus TCP directly rather than wrapping a library.
// The original FrostyGoop sample imported github.com/rolfl/modbus; we build the
// frames ourselves so the bytes on the wire can be diffed field by field
// against the sample's captures, which is one of the deliverables.
package modbus

import (
	"context"
	"encoding/binary"
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
	d := net.Dialer{Timeout: opt.Timeout}
	nc, err := d.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := &Conn{conn: nc, opt: opt}
	if opt.Timeout > 0 {
		nc.SetDeadline(time.Now().Add(opt.Timeout))
	}
	return c, nil
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

// sendRecv builds the MBAP + PDU, sends it, reads the response, validates the
// transaction ID, and returns the response PDU (without MBAP). It handles
// retries and timeouts per the Options.
func (c *Conn) sendRecv(ctx context.Context, pdu []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.opt.Retries; attempt++ {
		if attempt > 0 && c.opt.Debug {
			println("retry", attempt, "for tx", c.txID)
		}

		respPDU, err := c.doSendRecv(pdu)
		if err == nil {
			return respPDU, nil
		}
		lastErr = err

		// If it's a timeout or temporary network error, retry
		var netErr net.Error
		if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
			// Reconnect on network errors
			if c.conn != nil {
				c.conn.Close()
			}
			d := net.Dialer{Timeout: c.opt.Timeout}
			nc, dialErr := d.Dial("tcp", c.conn.RemoteAddr().String())
			if dialErr != nil {
				return nil, dialErr
			}
			c.conn = nc
			if c.opt.Timeout > 0 {
				nc.SetDeadline(time.Now().Add(c.opt.Timeout))
			}
			continue
		}
		// Non-retryable error
		break
	}
	return nil, lastErr
}

func (c *Conn) doSendRecv(pdu []byte) ([]byte, error) {
	c.txID++

	// Build MBAP header: transaction_id, protocol_id=0, length, unit_id
	mbap := make([]byte, 7)
	binary.BigEndian.PutUint16(mbap[0:2], c.txID)
	binary.BigEndian.PutUint16(mbap[2:4], 0) // protocol_id
	binary.BigEndian.PutUint16(mbap[4:6], uint16(len(pdu)+1))
	mbap[6] = c.opt.UnitID

	frame := append(mbap, pdu...)

	if c.opt.Debug {
		println("TX:", hexDump(frame))
	}

	if c.opt.Timeout > 0 {
		c.conn.SetDeadline(time.Now().Add(c.opt.Timeout))
	}
	if _, err := c.conn.Write(frame); err != nil {
		return nil, err
	}

	// Read MBAP header (7 bytes)
	mbapResp := make([]byte, 7)
	if _, err := readFull(c.conn, mbapResp); err != nil {
		return nil, err
	}

	respTxID := binary.BigEndian.Uint16(mbapResp[0:2])
	respProto := binary.BigEndian.Uint16(mbapResp[2:4])
	respLen := binary.BigEndian.Uint16(mbapResp[4:6])
	respUnitID := mbapResp[6]

	if respTxID != c.txID {
		return nil, errors.New("transaction ID mismatch")
	}
	if respProto != 0 {
		return nil, errors.New("invalid protocol ID in response")
	}
	if respUnitID != c.opt.UnitID {
		return nil, errors.New("unit ID mismatch")
	}
	if respLen < 2 {
		return nil, errors.New("response too short")
	}

	// Read PDU (respLen - 1 bytes, since unit_id is already in MBAP)
	pduLen := int(respLen) - 1
	pduResp := make([]byte, pduLen)
	if _, err := readFull(c.conn, pduResp); err != nil {
		return nil, err
	}

	if c.opt.Debug {
		println("RX:", hexDump(append(mbapResp, pduResp...)))
	}

	// Check for exception response
	if len(pduResp) >= 2 && pduResp[0]&0x80 != 0 {
		exceptionCode := pduResp[1]
		return nil, &ModbusError{FunctionCode: pduResp[0] & 0x7F, ExceptionCode: exceptionCode}
	}

	return pduResp, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	n := 0
	for n < len(buf) {
		nn, err := conn.Read(buf[n:])
		n += nn
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func hexDump(data []byte) string {
	hex := make([]byte, len(data)*3)
	for i, b := range data {
		hex[i*3] = "0123456789abcdef"[b>>4]
		hex[i*3+1] = "0123456789abcdef"[b&0x0f]
		if i < len(data)-1 {
			hex[i*3+2] = ' '
		}
	}
	return string(hex)
}

// ModbusError represents a Modbus exception response.
type ModbusError struct {
	FunctionCode  byte
	ExceptionCode byte
}

func (e *ModbusError) Error() string {
	return "modbus exception: function code 0x" + hexByte(e.FunctionCode) + " exception code 0x" + hexByte(e.ExceptionCode)
}

func hexByte(b byte) string {
	return "0123456789abcdef"[b>>4:b>>4+1] + "0123456789abcdef"[b&0x0f:b&0x0f+1]
}
