package modbus

import (
	"context"
	"encoding/binary"
	"errors"
)

// The five function codes the rewrite speaks. FC03/06/16 are what the sample
// had; FC01/FC05 are ours. That split is the project's central claim, so the
// coil calls stay visibly separate rather than folded into a generic helper.

// ReadCoils issues FC01.
func (c *Conn) ReadCoils(addr, count uint16) ([]bool, error) {
	if count < 1 || count > 2000 {
		return nil, errors.New("invalid coil count")
	}

	pdu := make([]byte, 5)
	pdu[0] = 0x01 // FC01
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], count)

	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return nil, err
	}

	if len(resp) < 2 {
		return nil, errors.New("invalid FC01 response length")
	}
	if resp[0] != 0x01 {
		return nil, errors.New("unexpected function code in response")
	}

	byteCount := int(resp[1])
	expectedBytes := (int(count) + 7) / 8
	if byteCount != expectedBytes {
		return nil, errors.New("unexpected byte count in FC01 response")
	}

	coils := make([]bool, count)
	data := resp[2:]
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
		return nil, errors.New("invalid register count")
	}

	pdu := make([]byte, 5)
	pdu[0] = 0x03 // FC03
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], count)

	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return nil, err
	}

	if len(resp) < 2 {
		return nil, errors.New("invalid FC03 response length")
	}
	if resp[0] != 0x03 {
		return nil, errors.New("unexpected function code in response")
	}

	byteCount := int(resp[1])
	if byteCount != int(count)*2 {
		return nil, errors.New("unexpected byte count in FC03 response")
	}

	values := make([]uint16, count)
	data := resp[2:]
	for i := 0; i < int(count); i++ {
		values[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
	}

	return values, nil
}

// WriteCoil issues FC05. On the wire a coil takes 0xFF00 or 0x0000, not 1/0.
func (c *Conn) WriteCoil(addr uint16, on bool) error {
	value := uint16(0x0000)
	if on {
		value = 0xFF00
	}

	pdu := make([]byte, 5)
	pdu[0] = 0x05 // FC05
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], value)

	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return err
	}

	if len(resp) != 5 {
		return errors.New("invalid FC05 response length")
	}
	if resp[0] != 0x05 {
		return errors.New("unexpected function code in response")
	}

	// Echo of request
	respAddr := binary.BigEndian.Uint16(resp[1:3])
	respValue := binary.BigEndian.Uint16(resp[3:5])
	if respAddr != addr || respValue != value {
		return errors.New("FC05 response does not match request")
	}

	return nil
}

// WriteSingle issues FC06.
func (c *Conn) WriteSingle(addr, value uint16) error {
	pdu := make([]byte, 5)
	pdu[0] = 0x06 // FC06
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], value)

	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return err
	}

	if len(resp) != 5 {
		return errors.New("invalid FC06 response length")
	}
	if resp[0] != 0x06 {
		return errors.New("unexpected function code in response")
	}

	// Echo of request
	respAddr := binary.BigEndian.Uint16(resp[1:3])
	respValue := binary.BigEndian.Uint16(resp[3:5])
	if respAddr != addr || respValue != value {
		return errors.New("FC06 response does not match request")
	}

	return nil
}

// WriteMultiple issues FC16.
func (c *Conn) WriteMultiple(addr uint16, values []uint16) error {
	count := len(values)
	if count < 1 || count > 123 {
		return errors.New("invalid register count")
	}

	byteCount := count * 2
	pdu := make([]byte, 6+byteCount)
	pdu[0] = 0x10 // FC16
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], uint16(count))
	pdu[5] = byte(byteCount)

	for i, v := range values {
		binary.BigEndian.PutUint16(pdu[6+i*2:6+i*2+2], v)
	}

	resp, err := c.sendRecv(context.Background(), pdu)
	if err != nil {
		return err
	}

	if len(resp) != 5 {
		return errors.New("invalid FC16 response length")
	}
	if resp[0] != 0x10 {
		return errors.New("unexpected function code in response")
	}

	respAddr := binary.BigEndian.Uint16(resp[1:3])
	respCount := binary.BigEndian.Uint16(resp[3:5])
	if respAddr != addr || respCount != uint16(count) {
		return errors.New("FC16 response does not match request")
	}

	return nil
}
