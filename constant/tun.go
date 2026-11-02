package constant

import (
	"errors"
	"strings"
)

var StackTypeMapping = map[string]TUNStack{
	strings.ToLower(TunGvisor.String()): TunGvisor,
	strings.ToLower(TunSystem.String()): TunSystem,
	strings.ToLower(TunMixed.String()):  TunMixed,
	strings.ToLower(TunMips.String()):   TunMips,
}

const (
	TunGvisor TUNStack = iota
	TunSystem
	TunMixed
	TunMips
)

type TUNStack int

// UnmarshalText unserialize TUNStack
func (e *TUNStack) UnmarshalText(data []byte) error {
	mode, exist := StackTypeMapping[strings.ToLower(string(data))]
	if !exist {
		return errors.New("invalid tun stack")
	}
	*e = mode
	return nil
}

// MarshalText serialize TUNStack with json
func (e TUNStack) MarshalText() ([]byte, error) {
	switch e {
	case TunGvisor:
		return tunStackTextGvisor, nil
	case TunSystem:
		return tunStackTextSystem, nil
	case TunMixed:
		return tunStackTextMixed, nil
	case TunMips:
		return tunStackTextMips, nil
	default:
		return tunStackTextUnknown, nil
	}
}

var (
	tunStackTextGvisor  = []byte("gVisor")
	tunStackTextSystem  = []byte("System")
	tunStackTextMixed   = []byte("Mixed")
	tunStackTextMips    = []byte("Mips")
	tunStackTextUnknown = []byte("unknown")
)

func (e TUNStack) String() string {
	switch e {
	case TunGvisor:
		return "gVisor"
	case TunSystem:
		return "System"
	case TunMixed:
		return "Mixed"
	case TunMips:
		return "Mips"
	default:
		return "unknown"
	}
}
