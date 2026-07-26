package attack

import (
	"errors"

	"github.com/abb00717/frostygoop-rewrite/internal/modbus"
)

// CoilTakeover drives the valves directly, bypassing the control loop. This
// path needs FC01/FC05, which the sample did not have, so it is only reachable
// in the rewrite.
//
// Sequence: read run_bit and abort unless TRUE, set manual_mode, write the four
// setpoints in one FC16, read back, roll manual_mode off on any failure. The
// run_bit precheck is not defensive padding, the ST forces the purge valve wide
// open while it is FALSE and the attack silently does nothing.
func CoilTakeover(c *modbus.Conn, f1, f2, purge, product uint16) (*Result, error) {
	return nil, errors.New("not implemented")
}
