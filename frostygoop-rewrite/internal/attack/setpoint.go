package attack

import (
	"errors"

	"github.com/abb00717/frostygoop-rewrite/internal/modbus"
)

// PressureSetpoint moves pressure_sp and lets the plant's own control loop
// raise the pressure. This is the only path the original sample could have
// taken here, since it reaches the goal with FC03/FC06 alone and never touches
// manual_mode, leaving the process looking normal throughout.
//
// Sequence: read the current value and keep it, write the new one, read back,
// restore the saved value on failure. Measured climb is roughly 35 raw units
// per second, so it takes three to four minutes to cross the threshold.
func PressureSetpoint(c *modbus.Conn, value uint16) (*Result, error) {
	return nil, errors.New("not implemented")
}
