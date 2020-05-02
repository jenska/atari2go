package cpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDisassembler(t *testing.T) {
	assert.NotNil(t, tcpu)
	assert.NotNil(t, trom)

	bus := tcpu.bus
	bra := bus.read(romTop, Word)
	assert.Equal(t, int32(0x602e), bra)
	d := Disassembler(romTop, bus)
	n := d.Next()
	assert.Equal(t, "00fc0000 bra.s      $00fc0030", n.String())

	d = Disassembler(0xfc0030, bus)
	//	assert.Equal(t, "00fc0030 move       #$2700, sr", d.Next().String())
}
