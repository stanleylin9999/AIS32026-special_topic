// Package attack orchestrates the two sequences the demo runs. Each sequence
// prechecks, writes, reads back, and rolls back on failure, mirroring the
// read/dispatch/report shape the sample's Task.taskWorker already had.
package attack

// PLC addresses, read off GRFICSv3/plc/st_files/326339.st. OpenPLC maps %QX
// bits to coils from 0 and %QW/%MW words to holding registers from 0 and 1024
// respectively, so %QX5.0 lands on coil 40 and %MW2 on HR1026.
const (
	CoilManualMode = 0  // manual_mode, %QX0.0
	CoilRunBit     = 40 // run_bit, %QX5.0

	// f1/f2/purge/product_manual_sp, %QW10-13. Only applied while manual_mode
	// is set, which is why the coil sequence has to set it first.
	HRManualSPBase = 10
	HRManualSPLen  = 4

	// pressure_sp, %MW2. The auto control loop never reassigns it, only clamps
	// it, so a write sticks and the loop drives the pressure up on its own.
	HRPressureSP = 1026

	PressureSPNormal = 55295 // 2700 kPa
	PressureSPMax    = 65535 // 3200 kPa, past the 3000 kPa damage threshold
)
