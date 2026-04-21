//go:build tinygo && esp32s3

// ---------------------------------------------------------------------------
// USB-Serial/JTAG output
// ---------------------------------------------------------------------------
package main

import (
	"unsafe"
	"time"
)

const (
	consoleLogBufferSize = 8 * 1024 //8kb
	usbFlushChunkBytes   = 32

	// usbWriteRetryLimit: retries of the full string if a byte write fails.
	// usbByteRetryLimit:  retries per byte before giving up on that byte.
	// usbRetryBackoff:    ms delay between byte retries (lets USB host drain).
	//
	// usbWriteByte spins waiting for SERIAL_IN_EP_DATA_FREE. At 240 MHz a
	// USB Full Speed frame is ~240 000 cycles so the timeout must be at least
	// that to survive one frame-period stall after a flush.
	usbWriteRetryLimit = 3
	usbByteRetryLimit  = 4
	usbRetryBackoff    = time.Duration(5) // 5 ms
)


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

var consoleLogBuffer [consoleLogBufferSize]byte
var consoleLogWrite int
var consoleLogFull bool

func usbWriteString(s string) bool {
	pending := 0
	for i := 0; i < len(s); i++ {
		if !usbWriteByteWithRetry(s[i]) {
			if pending > 0 {
				usbFlush()
			}
			return false
		}
		pending++
		if pending >= usbFlushChunkBytes {
			usbFlush()
			pending = 0
		}
	}
	if pending > 0 {
		usbFlush()
	}
	return true
}

func usbWriteBytes(p []byte) bool {
	pending := 0
	for i := 0; i < len(p); i++ {
		if !usbWriteByteWithRetry(p[i]) {
			if pending > 0 {
				usbFlush()
			}
			return false
		}
		pending++
		if pending >= usbFlushChunkBytes {
			usbFlush()
			pending = 0
		}
	}
	if pending > 0 {
		usbFlush()
	}
	return true
}

func usbWriteByteWithRetry(b byte) bool {
	for attempt := 0; attempt < usbByteRetryLimit; attempt++ {
		if usbWriteByte(b) {
			return true
		}
		busyDelay(usbRetryBackoff)
	}
	return false
}

func usbWriteByte(b byte) bool {
	// 1 500 000 iterations ≈ 6 ms at 240 MHz — enough to wait through several
	// USB Full Speed frame periods for the TX FIFO to drain after a flush.
	timeout := uint32(1_500_000)
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
	conf := usbEP1Conf.get()
	conf &^= usbWRDone
	usbEP1Conf.set(conf)
	usbEP1Conf.set(conf | usbWRDone)
}

// ---------------------------------------------------------------------------
// UART0 output (directly on hardware TX pin, independent of USB)
// ---------------------------------------------------------------------------

func uartWriteString(s string) {
	for i := 0; i < len(s); i++ {
		uartWriteByte(s[i])
	}
}

// consoleLogln buffers a line during boot. Call dumpConsoleLog once USB CDC
// has had time to enumerate (e.g. in the halt loop) to emit everything at once.
func consoleLogln(message string) {
	consoleLogAppend(message)
	consoleLogAppend("\r\n")
}

func consoleLogAppend(s string) {
	for i := 0; i < len(s); i++ {
		consoleLogBuffer[consoleLogWrite] = s[i]
		consoleLogWrite++
		if consoleLogWrite >= len(consoleLogBuffer) {
			consoleLogWrite = 0
			consoleLogFull = true
		}
	}
}

// dumpConsoleLog flushes the ring buffer over USB-Serial/JTAG.
// Returns true and resets the buffer on success so the next call is a no-op
// until new messages are appended. Retrying in the halt loop is intentional.
func dumpConsoleLog() bool {
	if !consoleLogFull && consoleLogWrite == 0 {
		return true
	}
	var ok bool
	if !consoleLogFull {
		ok = usbWriteBytes(consoleLogBuffer[:consoleLogWrite])
	} else {
		ok = usbWriteBytes(consoleLogBuffer[consoleLogWrite:]) &&
			usbWriteBytes(consoleLogBuffer[:consoleLogWrite])
	}
	if ok {
		consoleLogWrite = 0
		consoleLogFull = false
	}
	return ok
}

func uartWriteBuffer(p []byte) {
	for i := 0; i < len(p); i++ {
		uartWriteByte(p[i])
	}
}

// consoleWriteString routes kernel logs through USB-Serial/JTAG only.
// No UART fallback: UART does not appear in tinygo monitor (/dev/ttyACM0).
func consoleWriteString(s string) {
	for attempt := 0; attempt < usbWriteRetryLimit; attempt++ {
		if usbWriteString(s) {
			return
		}
		busyDelay(usbRetryBackoff)
	}
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

func busyDelay(ms time.Duration) {
	time.Sleep(time.Millisecond * ms)
	// for i := uint32(0); i < cycles; i++ {
	// 	// Prevent aggressive loop elimination.
	// 	_ = uart0Status.get()
	// }
}
