//go:build tinygo && esp32s3

package main

import "unsafe"
import "time"

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
	uarts = "uart"
	usbs = "usb"
)

type volatile32 struct{ v uint32 }

func (r *volatile32) get() uint32 {
	return *(*uint32)(unsafe.Pointer(&r.v))
}

func (r *volatile32) set(val uint32) {
	*(*uint32)(unsafe.Pointer(&r.v)) = val
}

var usbEP1 = (*volatile32)(unsafe.Pointer(usbSerialJTAGBase + usbEPDataOffset))
var usbEP1Conf = (*volatile32)(unsafe.Pointer(usbSerialJTAGBase + usbEPConfOffset))
var uart0FIFO = (*volatile32)(unsafe.Pointer(uart0Base + uartFifoOffset))
var uart0Status = (*volatile32)(unsafe.Pointer(uart0Base + uartStatusReg))

func main() {
	// Give the USB CDC host time to re-enumerate after the hard reset
	// performed by esptool.
    time.Sleep(time.Millisecond * 4000)

	consoleWriteString("windmills: phase0 boot\r\n")

	phase1Init(kernelThread0)
	consoleWriteString("windmills: phase1 init\r\n")

	phase2Init()
	consoleWriteString("windmills: phase2 init\r\n")

	schedulerRun()

	consoleWriteString("windmills: entering halt loop\r\n")

	// Write periodically so the monitor can be connected at any time
	// and still receive confirmation that the kernel reached halt.
	for {
		consoleWriteString("windmills: alive\r\n")
		time.Sleep(time.Millisecond * 1000)
		// busyDelay(2_000_000_000)
		// busyDelay(2_000_000_000)
	}
}

func kernelThread0() {
	consoleWriteString("windmills: kthread0 ran\r\n")
}

func halt() {
	for {
	}
}

// ---------------------------------------------------------------------------
// USB-Serial/JTAG output
// ---------------------------------------------------------------------------

func usbWriteString(s string) bool {
	for i := 0; i < len(s); i++ {
		if !usbWriteByte(s[i]) {
			return false
		}
	}
	usbFlush()
	return true
}

func usbWriteByte(b byte) bool {
	timeout := uint32(5_000_000)
	for usbEP1Conf.get()&usbSerialInEPDataFree == 0 {
		timeout--
		if timeout == 0 {
			return false
		}
	}
	usbEP1.set(uint32(b))
	return true
}

func usbFlush() {
	usbEP1Conf.set(usbEP1Conf.get() | usbWRDone)
}

// ---------------------------------------------------------------------------
// UART0 output (directly on hardware TX pin, independent of USB)
// ---------------------------------------------------------------------------

func uartWriteString(s string) {
	for i := 0; i < len(s); i++ {
		uartWriteByte(s[i])
	}
}

// consoleWriteString routes kernel logs through USB-Serial/JTAG.
func consoleWriteString(s string) {
	//_ = usbWriteString(usbs)
	_ = usbWriteString(s)
	//uartWriteString(uarts)
	//uartWriteString(s)
}

func uartWriteByte(b byte) {
	timeout := uint32(100000)
	for ((uart0Status.get() & uartTxFifoMask) >> 16) >= 126 {
		timeout--
		if timeout == 0 {
			return
		}
	}
	uart0FIFO.set(uint32(b))
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func busyDelay(cycles uint32) {
	for i := uint32(0); i < cycles; i++ {
		// Prevent aggressive loop elimination.
		_ = uart0Status.get()
	}
}
