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
	wrDone             uint32 = 1 << 0 // bit 0: flush TX FIFO
	serialInEPDataFree uint32 = 1 << 1 // bit 1: FIFO ready for write
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
	const timeout = 500_000
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

func busyDelay(n int) {
	x := 0;
	for i := 0; i < n; i++ {
		// Volatile read prevents the compiler from optimizing the loop away.
		readReg32(usbSerialBase + ep1ConfOffset)
		x++;
	}
	usbWriteString("test04: busyDelay\r\n");
}

func main() {
	// Initial delay to let USB host re-enumerate after reset.
	busyDelay(10_000_000)

	for {
		usbWriteString("test04: hello from USB\r\n")
		busyDelay(5_000_000)
	}
}
