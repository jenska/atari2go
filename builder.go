package cpu

import (
	"log"
	"strconv"
	"strings"
)

type (
	// AddressBusBuilder builds a AddressBus for M68K CPU
	AddressBusBuilder interface {
		Build() AddressBus
		AddArea(offset, size int32, area *AddressArea) AddressBusBuilder
	}

	// Builder builds a M68K CPU
	Builder interface {
		Build() *M68K
		SetBus(AddressBus) Builder
		SetISA68000() Builder
		// SetResetHandler(resetHandler chan<- struct{})
	}

	opcode struct {
		name                string
		instruction         instruction
		match, mask, eaMask uint16
		cycles              map[rune]*int
	}
)

var opcodeTable = []*opcode{}

// NewBuilder creates a new CPU Builder object
func NewBuilder() Builder {
	return &M68K{sr: ssr{S: true, Interrupts: 7}}
}

// SetBus sets the CPU's address bus
func (cpu *M68K) SetBus(bus AddressBus) Builder {
	if bus == nil {
		panic("bus must not be nil")
	}
	bus.read(0, Long) // check if the minimum amount of memory is available
	bus.read(4, Long)
	cpu.bus = bus
	return cpu
}

// Build routine starter for M68K
func (cpu *M68K) Build() *M68K {
	if cpu.bus == nil {
		panic("bus must not be nil")
	}
	cpu.Reset()
	return cpu
}

// NewAddressBusBuilder returns a new AddressBusBuilder
func NewAddressBusBuilder() AddressBusBuilder {
	return make(addressAreaTable, 0x10000)
}

// AddArea adds an address area to the address bus
func (aat addressAreaTable) AddArea(address, size int32, area *AddressArea) AddressBusBuilder {
	if area == nil {
		panic("AdressArea must not be nil")
	}
	if address < 0 || address&0xffff != 0 {
		panic("adddress must not be negative and multiple of 0x10000")
	}
	if size <= 0 || size&0xffff != 0 {
		panic("size must be multiple of 0x1000 and not less or equal 0")
	}

	handler := &addressAreaHandler{area, address, size}
	for i := address >> 16; i < (address+size)>>16; i++ {
		if aat[i] != nil {
			panic("address area already in use")
		}
		aat[i] = handler
	}

	return aat
}

func (aat addressAreaTable) Build() AddressBus {
	return aat
}

// NewAddressArea creates a new AddressArea with accessors read, write and resetHandler
// Build behaviour:
// read accessor is mandatory
// Runtime behaviour:
// If a write accessor is nil, the access to the address area will panic a BusError
// If reset is nil, no reset will be perfomed
func NewAddressArea(read Reader, write Writer, reset Reset) *AddressArea {
	if read == nil {
		panic("read accessor is mandatory")
	}
	return &AddressArea{
		read:  read,
		write: write,
		reset: reset,
	}
}

// NewROMArea returns a Read-Only, Pre-filled address area
func NewROMArea(mem []byte) *AddressArea {
	if mem == nil || len(mem) == 0 {
		panic("mem must not be nil or of size 0")
	}
	rom := mem
	return NewAddressArea(
		func(offset int32, s *Size) int32 {
			return s.read(rom[offset:])
		},
		nil,
		nil)
}

// NewRAMArea returns a RAM address area
func NewRAMArea(size uint32) *AddressArea {
	ram := make([]byte, size)
	return NewAddressArea(
		func(offset int32, s *Size) int32 {
			return s.read(ram[offset:])
		},
		func(offset int32, s *Size, value int32) {
			s.write(ram[offset:], value)
		},
		func() {
			for i := range ram {
				ram[i] = 0
			}
		},
	)
}

// NewBaseArea returns a new M68K base address area starting at address 0
func NewBaseArea(ssp, pc int32, size uint32) *AddressArea {
	if size < 8 {
		panic("size must be at least 8 bytes")
	}
	ram := make([]byte, size)
	Long.write(ram[0:], ssp)
	Long.write(ram[4:], pc)

	return NewAddressArea(
		func(offset int32, s *Size) int32 {
			return s.read(ram[offset:])
		},
		func(offset int32, s *Size, value int32) {
			if offset < 8 {
				panic(BusError)
			}
			s.write(ram[offset:], value)
		},
		func() {
			for i := 8; i < len(ram); i++ {
				ram[i] = 0
			}
		},
	)
}

// SetISA68000 Instruction Set Architecture for M68000
func (cpu *M68K) SetISA68000() Builder {
	cpu.eaDst = eaDst68000
	cpu.eaSrc = eaSrc68000

	cpu.d = cpu.da[:8]
	cpu.a = cpu.da[8:]

	c := cpu
	cpu.read = func(a int32, s *Size) int32 {
		if a&1 == 1 && s != Byte {
			panic(AdressError)
		}
		return c.bus.read(a&0x00ffffff, s)
	}

	cpu.write = func(a int32, s *Size, value int32) {
		if a&1 == 1 && s != Byte {
			panic(AdressError)
		}
		if !c.sr.S && a < 0x800 {
			panic(PrivilegeViolationError)
		}
		c.bus.write(a&0x00ffffff, s, value)
	}

	buildInstructionTable(c, '0')
	return cpu
}

func validEA(opcode, mask uint16) bool {
	if mask == 0 {
		return true
	}
	switch (opcode & 0x3f) >> 3 {
	case 0x00:
		return (mask & 0x800) != 0
	case 0x01:
		return (mask & 0x400) != 0
	case 0x02:
		return (mask & 0x200) != 0
	case 0x03:
		return (mask & 0x100) != 0
	case 0x04:
		return (mask & 0x080) != 0
	case 0x05:
		return (mask & 0x040) != 0
	case 0x06:
		switch opcode & 0x3f {
		case 0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37:
			return (mask & 0x020) != 0
		case 0x38:
			return (mask & 0x010) != 0
		case 0x39:
			return (mask & 0x008) != 0
		case 0x3a:
			return (mask & 0x002) != 0
		case 0x3b:
			return (mask & 0x001) != 0
		case 0x3c:
			return (mask & 0x004) != 0
		}
	}
	return false
}

func buildInstructionTable(c *M68K, r rune) {
	var counter int
	for _, opcode := range opcodeTable {
		match := opcode.match
		if opcode.cycles[r] != nil {
			mask := opcode.mask
			for value := uint16(0); ; {
				index := match | value

				if validEA(index, opcode.eaMask) {
					if c.instructions[index] != nil {
						//	log.Printf("instruction 0x%04x (%s) already set\n", index, opcode.name)
					} else {
						counter++
					}
					c.instructions[index] = opcode.instruction
					c.cycles[index] = *opcode.cycles[r]
				}

				value = ((value | mask) + 1) & ^mask

				if value == 0 {
					break
				}
			}
		}
	}
	log.Printf("%d cpu instructions available", counter)
}

func addOpcode(name string, ins instruction, match, mask uint16, eaMask uint16, cycles ...string) {
	// log.Printf("add opcode %s\n", name)
	cycleMap := map[rune]*int{}
	for _, entry := range cycles {
		parts := strings.Split(entry, ":")
		c := parts[1]
		cnt := toInt(c, 10)
		for _, r := range parts[0] {
			cycleMap[r] = &cnt
		}
	}
	opcodeTable = append(opcodeTable, &opcode{name, ins, match, mask, eaMask, cycleMap})
}

func toInt(s string, base int) int {
	if v, x := strconv.ParseInt(s, base, 64); x == nil {
		return int(v)
	}
	panic(x)
}
