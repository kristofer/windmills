//go:build tinygo && esp32s3

package main

import (
	"unsafe"
)

// USB-Serial/JTAG registers (base 0x60038000)
const usbSerialBase = 0x60038000

var usbEP1 = (*volatile32)(unsafe.Pointer(uintptr(usbSerialBase + 0x00)))
var usbEP1Conf = (*volatile32)(unsafe.Pointer(uintptr(usbSerialBase + 0x04)))

// UART0 registers (base 0x60000000)
const uart0Base = 0x60000000

var uart0FIFO = (*volatile32)(unsafe.Pointer(uintptr(uart0Base + 0x00)))
var uart0Status = (*volatile32)(unsafe.Pointer(uintptr(uart0Base + 0x1C)))

type volatile32 struct{ v uint32 }

func (r *volatile32) get() uint32 {
	return *(*uint32)(unsafe.Pointer(&r.v))
}

func (r *volatile32) set(val uint32) {
	*(*uint32)(unsafe.Pointer(&r.v)) = val
}

func busyDelay(cycles uint32) {
	for i := uint32(0); i < cycles; i++ {
		// Read from a known register to prevent the loop from being
		// optimized away.
		_ = uart0Status.get()
	}
}

// uart0TxFIFOCount returns the number of bytes currently in UART0's TX FIFO.
// STATUS register bits [23:16] hold TXFIFO_CNT.
func uart0TxFIFOCount() uint32 {
	return (uart0Status.get() >> 16) & 0xFF
}

func uart0WriteByte(b byte) {
	timeout := uint32(100000)
	for uart0TxFIFOCount() >= 126 {
		timeout--
		if timeout == 0 {
			return
		}
	}
	uart0FIFO.set(uint32(b))
}

func uart0WriteString(s string) {
	for i := 0; i < len(s); i++ {
		uart0WriteByte(s[i])
	}
}

// usbSerialReady returns true when the USB-Serial TX FIFO can accept data.
// Bit 1 of EP1_CONF = SERIAL_IN_EP_DATA_FREE.
func usbSerialReady() bool {
	return usbEP1Conf.get()&0x02 != 0
}

func usbWriteByte(b byte) {
	timeout := uint32(100000)
	for !usbSerialReady() {
		timeout--
		if timeout == 0 {
			return
		}
	}
	usbEP1.set(uint32(b))
}

func usbFlush() {
	// Set WR_DONE (bit 0) to flush the TX FIFO to the host.
	usbEP1Conf.set(usbEP1Conf.get() | 0x01)
}

func usbWriteString(s string) {
	for i := 0; i < len(s); i++ {
		usbWriteByte(s[i])
	}
	usbFlush()
}

// writeUint32 writes the decimal representation of n to both outputs.
// Uses a stack buffer to avoid heap allocation.
func writeUint32(n uint32) {
	var buf [10]byte
	pos := len(buf)
	if n == 0 {
		pos--
		buf[pos] = '0'
	} else {
		for n > 0 {
			pos--
			buf[pos] = byte('0' + n%10)
			n /= 10
		}
	}
	for i := pos; i < len(buf); i++ {
		uart0WriteByte(buf[i])
		usbWriteByte(buf[i])
	}
}

func dualWriteString(s string) {
	uart0WriteString(s)
	usbWriteString(s)
}

func main() {
	// Initial delay to let USB host enumerate.
	busyDelay(5000000)

	counter := uint32(0)
	for {
		// Write the message in parts to avoid string concatenation
		// (which would require runtime.alloc, unavailable with gc=none).
		uart0WriteString("test05: dual output ")
		usbWriteString("test05: dual output ")
		writeUint32(counter)
		uart0WriteString("\r\n")
		usbWriteString("\r\n")
		usbFlush()

		counter++
		busyDelay(10000000)
	}
}
