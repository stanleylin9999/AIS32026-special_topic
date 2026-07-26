package attack

import (
	"errors"
	"testing"
	"time"
)

type fakeClient struct {
	coils            map[uint16]bool
	holding          map[uint16]uint16
	readCoilsQueue   [][]bool
	readHoldingQueue [][]uint16
	writeCoilCalls   []bool
	writeMultipleErr error
	writeSingleErr   error
	afterWriteSingle func(addr, value uint16)
}

func (f *fakeClient) ReadCoils(addr, count uint16) ([]bool, error) {
	if len(f.readCoilsQueue) > 0 {
		v := f.readCoilsQueue[0]
		f.readCoilsQueue = f.readCoilsQueue[1:]
		return v, nil
	}
	out := make([]bool, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.coils[addr+i]
	}
	return out, nil
}

func (f *fakeClient) ReadHolding(addr, count uint16) ([]uint16, error) {
	if len(f.readHoldingQueue) > 0 {
		v := f.readHoldingQueue[0]
		f.readHoldingQueue = f.readHoldingQueue[1:]
		return v, nil
	}
	out := make([]uint16, count)
	for i := uint16(0); i < count; i++ {
		out[i] = f.holding[addr+i]
	}
	return out, nil
}

func (f *fakeClient) WriteCoil(addr uint16, on bool) error {
	f.coils[addr] = on
	f.writeCoilCalls = append(f.writeCoilCalls, on)
	return nil
}

func (f *fakeClient) WriteMultiple(addr uint16, values []uint16) error {
	if f.writeMultipleErr != nil {
		return f.writeMultipleErr
	}
	for i, v := range values {
		f.holding[addr+uint16(i)] = v
	}
	return nil
}

func (f *fakeClient) WriteSingle(addr, value uint16) error {
	if f.writeSingleErr != nil {
		return f.writeSingleErr
	}
	f.holding[addr] = value
	if f.afterWriteSingle != nil {
		f.afterWriteSingle(addr, value)
	}
	return nil
}

func TestCoilTakeoverSuccess(t *testing.T) {
	fc := &fakeClient{coils: map[uint16]bool{}, holding: map[uint16]uint16{}}
	fc.coils[CoilRunBit] = true

	res, err := coilTakeover(fc, 10, 11, 12, 13, 10*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success")
	}
	if res.RolledBack {
		t.Fatalf("did not expect rollback")
	}
	if got := fc.holding[HRManualSPBase+3]; got != 13 {
		t.Fatalf("unexpected holding write: got=%d", got)
	}
}

func TestCoilTakeoverRunBitTimeout(t *testing.T) {
	fc := &fakeClient{coils: map[uint16]bool{}, holding: map[uint16]uint16{}}
	fc.coils[CoilRunBit] = false

	res, err := coilTakeover(fc, 1, 2, 3, 4, 3*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if res.Success {
		t.Fatalf("expected failure")
	}
	if len(fc.writeCoilCalls) != 0 {
		t.Fatalf("manual_mode should not be written when run_bit never turns true")
	}
}

func TestCoilTakeoverWriteMultipleRollback(t *testing.T) {
	fc := &fakeClient{coils: map[uint16]bool{}, holding: map[uint16]uint16{}}
	fc.coils[CoilRunBit] = true
	fc.writeMultipleErr = errors.New("boom")

	res, err := coilTakeover(fc, 1, 2, 3, 4, 10*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatalf("expected write failure")
	}
	if !res.RolledBack {
		t.Fatalf("expected rollback")
	}
	if len(fc.writeCoilCalls) < 2 || fc.writeCoilCalls[0] != true || fc.writeCoilCalls[1] != false {
		t.Fatalf("expected manual_mode on then off rollback, calls=%v", fc.writeCoilCalls)
	}
}

func TestPressureSetpointSuccess(t *testing.T) {
	fc := &fakeClient{coils: map[uint16]bool{}, holding: map[uint16]uint16{}}
	fc.holding[HRPressureSP] = 55295

	res, err := pressureSetpoint(fc, 65535)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success when write+verify match")
	}
}

func TestPressureSetpointRollbackOnMismatch(t *testing.T) {
	fc := &fakeClient{coils: map[uint16]bool{}, holding: map[uint16]uint16{}}
	fc.holding[HRPressureSP] = 55295
	mismatchInjected := false
	fc.afterWriteSingle = func(addr, value uint16) {
		if addr == HRPressureSP && !mismatchInjected {
			mismatchInjected = true
			fc.holding[addr] = 12345
		}
	}

	res, err := pressureSetpoint(fc, 65535)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !res.RolledBack {
		t.Fatalf("expected rollback")
	}
	if got := fc.holding[HRPressureSP]; got != 55295 {
		t.Fatalf("expected rollback to 55295, got %d", got)
	}
}
