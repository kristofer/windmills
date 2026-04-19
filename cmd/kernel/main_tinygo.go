//go:build tinygo && esp32s3

package main

import "unsafe"

// USB-Serial/JTAG controller registers (base 0x6003_8000)
const (
	usbSerialJTAGBase = uintptr(0x60038000)
	usbEPDataOffset   = uintptr(0x00) // USB_SERIAL_JTAG_EP1_REG (TX FIFO write)
	usbEPConfOffset   = uintptr(0x04) // USB_SERIAL_JTAG_EP1_CONF_REG

	// EP1_CONF bits
	usbWRDone             = uint32(0x1) // bit 0: flush TX FIFO to host
	usbSerialInEPDataFree = uint32(0x2) // bit 1: TX FIFO has space

	// UART0 registers (fallback output on hardware TX pin)
	uart0Base      = uintptr(0x60000000)
	uartFifoOffset = uintptr(0x00)
	uartStatusReg  = uintptr(0x1C)
	uartTxFifoMask = uint32(0xFF0000)
)

func main() {
	// Give the USB CDC host time to re-enumerate after the hard reset
	// performed by esptool.  ~500 ms at 240 MHz.
	spin(120_000_000)

	msg := "windmills: alive\r\n"

	// Write forever so the user can open the monitor at any time and
	// still see output.
	for {
		// Try both USB-Serial/JTAG and UART0 so output appears
		// regardless of which port the user monitors.
		usbWriteString(msg)
		uartWriteString(msg)

		phase1Init(kernelThread0)
		phase2Init()
		schedulerRun()

		msg = "windmills: loop\r\n"
		spin(120_000_000) // ~500 ms between iterations
	}
}

func kernelThread0() {
	usbWriteString("windmills: kthread0 ran\r\n")
	uartWriteString("windmills: kthread0 ran\r\n")
}

func halt() {
	for {
	}
}

// ---------------------------------------------------------------------------
// USB-Serial/JTAG output
// ---------------------------------------------------------------------------

func usbWriteString(s string) {
	for i := 0; i < len(s); i++ {
		usbWriteByte(s[i])
	}
	usbFlush()
}

func usbWriteByte(b byte) {
	conf := usbSerialJTAGBase + usbEPConfOffset
	// Spin until FIFO has space (with a generous timeout so we never hang
	// forever if the host side is not listening).
	for i := 0; i < 1_000_000; i++ {
		if *(*uint32)(unsafe.Pointer(conf))&usbSerialInEPDataFree != 0 {
			*(*uint32)(unsafe.Pointer(usbSerialJTAGBase + usbEPDataOffset)) = uint32(b)
			return
		}
	}
	// Timeout — just drop the byte.
}

func usbFlush() {
	conf := usbSerialJTAGBase + usbEPConfOffset
	v := *(*uint32)(unsafe.Pointer(conf))
	*(*uint32)(unsafe.Pointer(conf)) = v | usbWRDone
}

// ---------------------------------------------------------------------------
// UART0 output (directly on hardware TX pin, independent of USB)
// ---------------------------------------------------------------------------

func uartWriteString(s string) {
	for i := 0; i < len(s); i++ {
		uartWriteByte(s[i])
	}
}

func uartWriteByte(b byte) {
	for {
		status := *(*uint32)(unsafe.Pointer(uart0Base + uartStatusReg))
		if ((status & uartTxFifoMask) >> 16) < 127 {
			break
		}
	}
	*(*uint32)(unsafe.Pointer(uart0Base + uartFifoOffset)) = uint32(b)
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

// spin performs a busy-wait loop for roughly n iterations.
//
//go:noinline
func spin(n int) {
	for i := 0; i < n; i++ {
		nop()
	}
}

//go:noinline
func nop() {}
