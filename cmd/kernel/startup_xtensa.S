//go:build tinygo && esp32s3

// Minimal startup stubs for Phase 0 bring-up. The linker entrypoint is _start.

.globl _start
_start:
j kmain

.globl windmills_vector_stub
windmills_vector_stub:
j windmills_vector_stub
