package attack

// Step records one Modbus operation. The operator reads these back in the
// Mythic UI, so they double as the demo's on-screen evidence and have to name
// the PLC variable, not just the address.
type Step struct {
	Name  string   `json:"name"`
	FC    byte     `json:"fc"`
	Addr  uint16   `json:"addr"`
	Value []uint16 `json:"value,omitempty"`
	Err   string   `json:"err,omitempty"`
}

type Result struct {
	Steps      []Step `json:"steps"`
	Success    bool   `json:"success"`
	RolledBack bool   `json:"rolled_back"`
}
