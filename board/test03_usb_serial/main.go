//go:build tinygo && esp32s3

package main

import "unsafe"

// USB-Serial/JTAG controller base address
const usbSerialBase uintptr = 0x60038000

// Register offsets
const (
	ep1Offset     uintptr = 0x00 // EP1 TX FIFO data write (8-bit)
	ep1ConfOffset uintptr = 0x04 // EP1_CONF
)

// EP1_CONF bits
const (
	wrDone              uint32 = 1 << 0 // bit 0: flush TX FIFO
	serialInEPDataFree  uint32 = 1 << 1 // bit 1: FIFO ready for write
)

func writeReg32(addr uintptr, val uint32) {
	*(*uint32)(unsafe.Pointer(addr)) = val
}

func readReg32(addr uintptr) uint32 {
	return *(*uint32)(unsafe.Pointer(addr))
}

// waitFIFOReady spins until the TX FIFO is ready or timeout expires.
// Returns true if ready, false on timeout.
func waitFIFOReady() bool {
	const timeout = 500000
	for i := 0; i < timeout; i++ {
		if readReg32(usbSerialBase+ep1ConfOffset)&serialInEPDataFree != 0 {
			return true
		}
	}
	return false
}

func usbWriteByte(b byte) bool {
	if !waitFIFOReady() {
		return false
	}
	writeReg32(usbSerialBase+ep1Offset, uint32(b))
	return true
}

func usbFlush() {
	writeReg32(usbSerialBase+ep1ConfOffset, wrDone)
}

func usbWriteString(s string) {
	for i := 0; i < len(s); i++ {
		if !usbWriteByte(s[i]) {
			return
		}
	}
	usbFlush()
}

func busyDelay() {
	for i := 0; i < 5_000_000; i++ {
		// Prevent the compiler from optimizing this away.
		// TinyGo's arm.Asm is not available on Xtensa, so we use a
		// volatile read instead.
		readReg32(usbSerialBase + ep1ConfOffset)
	}
}

func main() {
	// Initial delay to let USB host enumerate the device.
	busyDelay()
	busyDelay()

	for {
		usbWriteString("test03: hello from USB\r\n")
		busyDelay()
	}
}
