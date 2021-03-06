package cpu

func init() {
	addOpcode("bcc", bcc, 0x6000, 0xf000, 0x0000, "01:10", "7:13", "234fc:6")
	addOpcode("dbcc", dbcc, 0x50c8, 0xf0f8, 0x0000, "0:12", "7:14", "1:10", "234fc:6")
	addOpcode("rts", rts, 0x4e75, 0xffff, 0x0000, "01:16", "7:15", "234fc:10")
	addOpcode("rtr", rtr, 0x4e77, 0xffff, 0x0000, "01:20", "7:22", "234fc:14")
	addOpcode("jmp ea", jmp, 0x4ec0, 0xffc0, 0x27b, "01:4", "7:7", "234fc:0")
	addOpcode("rte  ; 68000", rte68000, 0x4e73, 0xffff, 0x0000, "0:20")
	addOpcode("trap #imm", trap, 0x4e40, 0xfff0, 0x000, "071234fc:4")
}

func bcc(c *M68K) {
	cc := (c.ir >> 8) & 0xf
	dis := int32(c.ir & 0xff)
	if cc == 1 { // bsr
		if dis == 0 {
			dis = int32(int16(c.popPc(Word)))
		} else {
			dis = int32(int8(dis))
		}
		c.push(Long, c.pc)
		c.pc += dis
		// c.cycles +=
	} else if c.sr.testCC(cc) {
		if dis == 0 {
			dis = int32(int16(c.popPc(Word)))
		} else {
			dis = int32(int8(dis))
		}
		c.pc += dis
		// c.cycles +=
	} else {
		// c.cycles +=
		if dis == 0 {
			c.pc += Word.size
		}
	}
}

func dbcc(c *M68K) {
	if c.sr.testCC(c.ir >> 8) {
		c.pc += Word.size // skip displacement value
		// c.cycles +=
	} else {
		count := int32(int16(*dy(c))) - 1
		Word.set(count, dy(c))
		dis := int32(int16(c.popPc(Word)))
		if count == -1 {
			// c.cycles +=
		} else {
			// c.cycles +=
			c.pc += dis - Word.size
		}
	}
}

func rts(c *M68K) {
	c.pc = c.pop(Long)
}

func rtr(c *M68K) {
	c.sr.setccr(c.pop(Word))
	c.pc = c.pop(Long)
}

func jmp(c *M68K) {
	c.pc = c.resolveDstEA(Long).computedAddress()
}

func rte68000(c *M68K) {
	if c.sr.S {
		newSR := c.pop(Word)
		newPC := c.pop(Long)
		c.pc = newPC
		c.sr.setbits(newSR)
	} else {
		panic(NewError(PrivilegeViolationError, c, c.pc, nil))
	}
}

func trap(c *M68K) {
	vec := int32(c.ir & 0xf)
	panic(NewError(TrapBase, c, c.pc, &vec))
}
