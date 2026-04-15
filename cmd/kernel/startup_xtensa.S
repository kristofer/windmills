//go:build tinygo && esp32s3

// Minimal startup stubs for Phase 0 bring-up. The linker entrypoint is _start.

.globl _start
_start:
	// Stack setup is intentionally deferred to TinyGo/ROM bring-up for this
	// minimal Phase 0 stub. This file only provides a deterministic entry symbol.
	// Expected CPU state contract at `kmain`: primary core only, ROM-established
	// early boot register defaults, interrupts effectively masked by boot flow,
	// and no dependency on cache/MMU reconfiguration in this phase.
	j kmain

.globl windmills_vector_stub
windmills_vector_stub:
j windmills_vector_stub
