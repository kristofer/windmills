//go:build tinygo && esp32s3

package main

import (
	"unsafe"
	_ "unsafe"
)

// Link TinyGo runtime allocation entrypoints to the kernel allocator.

//go:linkname runtimeAlloc runtime.alloc
func runtimeAlloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer {
	_ = layout
	return unsafe.Pointer(tinygoRuntimeAlloc(size))
}

//go:linkname runtimeRealloc runtime.realloc
func runtimeRealloc(ptr unsafe.Pointer, size uintptr) unsafe.Pointer {
	return unsafe.Pointer(tinygoRuntimeRealloc(uintptr(ptr), size))
}
