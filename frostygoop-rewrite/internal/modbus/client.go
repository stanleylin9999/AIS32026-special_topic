package modbus

// The five function codes the rewrite speaks. FC03/06/16 are what the sample
// had; FC01/FC05 are ours. That split is the project's central claim, so the
// coil calls stay visibly separate rather than folded into a generic helper.

// ReadCoils issues FC01.
func (c *Conn) ReadCoils(addr, count uint16) ([]bool, error) {
	return nil, errNotImplemented
}

// ReadHolding issues FC03.
func (c *Conn) ReadHolding(addr, count uint16) ([]uint16, error) {
	return nil, errNotImplemented
}

// WriteCoil issues FC05. On the wire a coil takes 0xFF00 or 0x0000, not 1/0.
func (c *Conn) WriteCoil(addr uint16, on bool) error {
	return errNotImplemented
}

// WriteSingle issues FC06.
func (c *Conn) WriteSingle(addr, value uint16) error {
	return errNotImplemented
}

// WriteMultiple issues FC16.
func (c *Conn) WriteMultiple(addr uint16, values []uint16) error {
	return errNotImplemented
}
