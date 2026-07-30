package modbus

import (
	"context"
	"encoding/binary"
	"errors"
)

var (
	errInvalidCount    = errors.New("invalid count")
	errInvalidAddress  = errors.New("invalid address")
	errInvalidValue    = errors.New("invalid value")
	errResponseTooShort = errors.New("response too short")
)

// The five function codes the rewrite speaks. FC03/06/16 are what the sample
// had; FC01/FC05 are ours. That split is the project's central claim, so the
// coil calls stay visibly separate rather than folded into a generic helper.

// ReadCoils issues FC01.
func (c *Conn) ReadCoils(addr, count uint16) ([]bool, error) {
	if count < 1 || count > 2000 {
		return nil, errInvalidCount
	}
	pdu := []byte{0x01, byte(addr >> 8), byte(addr), byte(count >> 8), byte(count)}
	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return nil, err
	}
	if len(resp) < 2 {
		return nil, errResponseTooShort
	}
	if resp[0] != 0x01 {
		return nil, errors.New("function code mismatch in response")
	}
	byteCount := int(resp[1])
	if len(resp) != 2+byteCount {
		return nil, errors.New("truncated response body")
	}
	data := resp[2:]
	coils := make([]bool, count)
	for i := 0; i < int(count); i++ {
		byteIdx := i / 8
		bitIdx := i % 8
		if byteIdx < len(data) {
			coils[i] = (data[byteIdx] & (1 << bitIdx)) != 0
		}
	}
	return coils, nil
}

// ReadHolding issues FC03.
func (c *Conn) ReadHolding(addr, count uint16) ([]uint16, error) {
	if count < 1 || count > 125 {
		return nil, errInvalidCount
	}
	pdu := []byte{0x03, byte(addr >> 8), byte(addr), byte(count >> 8), byte(count)}
	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return nil, err
	}
	if len(resp) < 2 {
		return nil, errResponseTooShort
	}
	if resp[0] != 0x03 {
		return nil, errors.New("function code mismatch in response")
	}
	byteCount := int(resp[1])
	if len(resp) != 2+byteCount || byteCount != int(count)*2 {
		return nil, errors.New("truncated response body")
	}
	data := resp[2:]
	regs := make([]uint16, count)
	for i := 0; i < int(count); i++ {
		regs[i] = binary.BigEndian.Uint16(data[i*2:])
	}
	return regs, nil
}

// WriteCoil issues FC05. On the wire a coil takes 0xFF00 or 0x0000, not 1/0.
func (c *Conn) WriteCoil(addr uint16, on bool) error {
	val := uint16(0x0000)
	if on {
		val = 0xFF00
	}
	pdu := []byte{0x05, byte(addr >> 8), byte(addr), byte(val >> 8), byte(val)}
	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return err
	}
	if len(resp) != 5 {
		return errResponseTooShort
	}
	if resp[0] != 0x05 || binary.BigEndian.Uint16(resp[1:3]) != addr || binary.BigEndian.Uint16(resp[3:5]) != val {
		return errors.New("write coil response mismatch")
	}
	return nil
}

// WriteSingle issues FC06.
func (c *Conn) WriteSingle(addr, value uint16) error {
	pdu := []byte{0x06, byte(addr >> 8), byte(addr), byte(value >> 8), byte(value)}
	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return err
	}
	if len(resp) != 5 {
		return errResponseTooShort
	}
	if resp[0] != 0x06 || binary.BigEndian.Uint16(resp[1:3]) != addr || binary.BigEndian.Uint16(resp[3:5]) != value {
		return errors.New("write single response mismatch")
	}
	return nil
}

// WriteMultiple issues FC16.
func (c *Conn) WriteMultiple(addr uint16, values []uint16) error {
	count := uint16(len(values))
	if count < 1 || count > 123 {
		return errInvalidCount
	}
	byteCount := count * 2
	pdu := make([]byte, 6+byteCount)
	pdu[0] = 0x10
	pdu[1] = byte(addr >> 8)
	pdu[2] = byte(addr)
	pdu[3] = byte(count >> 8)
	pdu[4] = byte(count)
	pdu[5] = byte(byteCount)
	for i, v := range values {
		binary.BigEndian.PutUint16(pdu[6+i*2:], v)
	}
	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return err
	}
	if len(resp) != 5 {
		return errResponseTooShort
	}
	if resp[0] != 0x10 || binary.BigEndian.Uint16(resp[1:3]) != addr || binary.BigEndian.Uint16(resp[3:5]) != count {
		return errors.New("write multiple response mismatch")
	}
	return nil
}

// ReadInput issues FC04.
func (c *Conn) ReadInput(addr, count uint16) ([]uint16, error) {
	if count < 1 || count > 125 {
		return nil, errInvalidCount
	}
	pdu := []byte{0x04, byte(addr >> 8), byte(addr), byte(count >> 8), byte(count)}
	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return nil, err
	}
	if len(resp) < 2 {
		return nil, errResponseTooShort
	}
	if resp[0] != 0x04 {
		return nil, errors.New("function code mismatch in response")
	}
	byteCount := int(resp[1])
	if len(resp) != 2+byteCount || byteCount != int(count)*2 {
		return nil, errors.New("truncated response body")
	}
	data := resp[2:]
	regs := make([]uint16, count)
	for i := 0; i < int(count); i++ {
		regs[i] = binary.BigEndian.Uint16(data[i*2:])
	}
	return regs, nil
}
