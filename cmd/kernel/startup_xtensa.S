//go:build tinygo && esp32s3

// Minimal startup stubs for Phase 0 bring-up. The linker entrypoint is _start.

.globl _start
_start:
	// Stack setup remains deferred to ROM/TinyGo boot flow in this skeleton.
	j kmain

.globl windmills_vector_table
windmills_vector_table:
	j windmills_reset_vector
	j windmills_level1_vector
	j windmills_level2_vector
	j windmills_level3_vector
	j windmills_level4_vector
	j windmills_level5_vector
	j windmills_double_exception_vector

.globl windmills_reset_vector
windmills_reset_vector:
	j windmills_trap_entry

.globl windmills_level1_vector
windmills_level1_vector:
	j windmills_trap_entry

.globl windmills_level2_vector
windmills_level2_vector:
	j windmills_trap_entry

.globl windmills_level3_vector
windmills_level3_vector:
	j windmills_trap_entry

.globl windmills_level4_vector
windmills_level4_vector:
	j windmills_trap_entry

.globl windmills_level5_vector
windmills_level5_vector:
	j windmills_trap_entry

.globl windmills_double_exception_vector
windmills_double_exception_vector:
	j windmills_trap_entry

.globl windmills_trap_entry
windmills_trap_entry:
	j windmills_trap_dispatch

.globl windmills_trap_dispatch
windmills_trap_dispatch:
	j windmills_trap_exit

.globl windmills_trap_exit
windmills_trap_exit:
	j windmills_trap_return

.globl windmills_trap_return
windmills_trap_return:
	j kmain
