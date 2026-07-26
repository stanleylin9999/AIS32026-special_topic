// Command frostygoop is the rewrite's CLI. The flag set tracks the sample's
// main.Cmd so the two can be compared directly; the extra -mode values are
// where our added capability shows up at the interface level.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
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
	mode    string
	address uint
	value   uint
	count   uint
	threads int
	timeout time.Duration
	try     int
	output  string
	debug   bool
}

func parseFlags() *config {
	c := &config{}
	flag.StringVar(&c.ip, "ip", "", "target host:port")
	flag.StringVar(&c.mode, "mode", "read", "operation, see the mode table")
	flag.UintVar(&c.address, "address", 0, "register or coil address")
	flag.UintVar(&c.value, "value", 0, "value to write")
	flag.UintVar(&c.count, "count", 1, "how many to read")
	flag.IntVar(&c.threads, "threads", 3, "worker count")
	flag.DurationVar(&c.timeout, "timeout", 10*time.Second, "per-request timeout")
	flag.IntVar(&c.try, "try", 3, "retries per request")
	flag.StringVar(&c.output, "output", "", "write the JSON result here")
	flag.BoolVar(&c.debug, "debug", false, "log frames as hex")
	flag.Parse()
	return c
}

func main() {
	parseFlags()
	fmt.Fprintln(os.Stderr, "not implemented")
	os.Exit(1)
}
