//go:build tinygo && esp32s3

// Minimal startup stubs for Phase 0 bring-up. The linker entrypoint is _start.

.globl _start
_start:
	// Stack setup is intentionally deferred to TinyGo/ROM bring-up for this
	// minimal Phase 0 stub. This file only provides a deterministic entry symbol.
	j kmain

.globl windmills_vector_stub
windmills_vector_stub:
j windmills_vector_stub
