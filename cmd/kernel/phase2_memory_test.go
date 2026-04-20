package main

import (
	"testing"
	"unsafe"
)

func configureHostBackedPhysicalMemory(t *testing.T) []byte {
	t.Helper()
	ram := make([]byte, int(physicalMemorySizeBytes)+int(pageSizeBytes))
	base := uintptr(unsafe.Pointer(&ram[0]))
	physicalMemoryBase = alignUp(base, pageSizeBytes)
	return ram
}

func resetPhase2ForTest() {
	physicalMemoryBase = 0x20000000
	regionTableCount = 0
	mem_init_complete = false
	bootAllocatorEnabled = false
	bootAllocatorPermanentlyDisabled = false
	bootAllocatorCursor = 0
	bootAllocatorEnd = 0
	nextFreeFrame = 0
	userNextFreeFrame = 0
	kernelPoolStartFrame = 0
	kernelPoolEndFrame = 0
	userPoolStartFrame = 0
	userPoolEndFrame = 0
	for i := range runtimeHeapAllocs {
		runtimeHeapAllocs[i] = runtimeHeapAllocation{}
	}
	for i := range pageFrameBitmap {
		pageFrameBitmap[i] = 0
	}
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	fn()
}

func TestPhase2InitBuildsMemoryMapAndDisablesBootAllocator(t *testing.T) {
	resetPhase2ForTest()

	phase2Init()

	if regionTableCount == 0 {
		t.Fatalf("memory map should be initialized")
	}
	if !bootAllocatorPermanentlyDisabled {
		t.Fatalf("boot allocator should be permanently disabled after phase2Init")
	}
	if bootAllocatorEnabled {
		t.Fatalf("boot allocator should be disabled after phase2Init")
	}
	if !mem_init_complete {
		t.Fatalf("phase2Init should enable mem_init_complete")
	}

	assertPanics(t, func() {
		bootAlloc(pageSizeBytes)
	})
}

func TestBootAllocatorRejectsReenableAfterDisable(t *testing.T) {
	resetPhase2ForTest()

	bootAllocatorInit(physicalMemoryBase)
	_ = bootAlloc(pageSizeBytes)
	bootAllocatorDisable()

	assertPanics(t, func() {
		bootAllocatorInit(physicalMemoryBase)
	})
}

func TestPageAllocatorNeverReturnsReservedRegions(t *testing.T) {
	resetPhase2ForTest()
	phase2Init()

	reservedStart := physicalMemoryBase
	reservedEnd := physicalMemoryBase + bootAllocatorReserveSize

	wantPages := int((kernelPoolEndFrame - kernelPoolStartFrame))
	gotPages := 0

	for {
		address, ok := allocPage()
		if !ok {
			break
		}
		gotPages++

		if address < physicalMemoryBase || address >= physicalMemoryBase+physicalMemorySizeBytes {
			t.Fatalf("allocated page outside physical memory: 0x%x", address)
		}
		if address >= reservedStart && address < reservedEnd {
			t.Fatalf("allocated page in reserved boot region: 0x%x", address)
		}
		if isReservedAddress(address) {
			t.Fatalf("allocated reserved address: 0x%x", address)
		}
	}

	if gotPages != wantPages {
		t.Fatalf("allocated %d pages, want %d", gotPages, wantPages)
	}
}

func TestPageAllocatorUsesConfigurableBaseAddress(t *testing.T) {
	resetPhase2ForTest()
	physicalMemoryBase = 0x28000000
	phase2Init()

	address, ok := allocPage()
	if !ok {
		t.Fatalf("expected at least one allocatable page")
	}
	want := physicalMemoryBase + uintptr(kernelPoolStartFrame)*pageSizeBytes
	if address != want {
		t.Fatalf("first alloc page = 0x%x, want 0x%x", address, want)
	}
}

func TestAllocContigAllocatesContiguousKernelPages(t *testing.T) {
	resetPhase2ForTest()
	phase2Init()

	address, ok := AllocContig(3)
	if !ok {
		t.Fatalf("expected contiguous allocation")
	}
	want := physicalMemoryBase + uintptr(kernelPoolStartFrame)*pageSizeBytes
	if address != want {
		t.Fatalf("AllocContig(3) = 0x%x, want 0x%x", address, want)
	}

	for i := uintptr(0); i < 3; i++ {
		page := address + i*pageSizeBytes
		if !FreePage(page) {
			t.Fatalf("expected FreePage to release page 0x%x", page)
		}
	}
}

func TestRuntimeHeapAllocationGuardedByMemInitComplete(t *testing.T) {
	resetPhase2ForTest()
	phase2Init()
	mem_init_complete = false

	if address, ok := runtimeHeapAlloc(pageSizeBytes); ok || address != 0 {
		t.Fatalf("runtimeHeapAlloc should fail when mem_init_complete=false")
	}

	mem_init_complete = true
	address, ok := runtimeHeapAlloc(pageSizeBytes)
	if !ok {
		t.Fatalf("runtimeHeapAlloc should succeed when mem_init_complete=true")
	}
	if !runtimeHeapFree(address) {
		t.Fatalf("runtimeHeapFree should free allocated runtime page")
	}
}

func TestRuntimeHeapFreeReleasesContiguousAllocation(t *testing.T) {
	resetPhase2ForTest()
	phase2Init()

	size := uintptr(3) * pageSizeBytes
	address, ok := runtimeHeapAlloc(size)
	if !ok {
		t.Fatalf("runtimeHeapAlloc should allocate contiguous pages")
	}
	if !runtimeHeapFree(address) {
		t.Fatalf("runtimeHeapFree should release contiguous allocation")
	}

	address2, ok := runtimeHeapAlloc(size)
	if !ok {
		t.Fatalf("runtimeHeapAlloc should allocate again after free")
	}
	if address2 != address {
		t.Fatalf("reallocated base = 0x%x, want 0x%x", address2, address)
	}
}

func TestTinygoRuntimeAllocPanicsOnFailure(t *testing.T) {
	resetPhase2ForTest()
	assertPanics(t, func() {
		_ = tinygoRuntimeAlloc(pageSizeBytes)
	})
}

func TestTinygoRuntimeAllocZeroesReturnedMemory(t *testing.T) {
	resetPhase2ForTest()
	ram := configureHostBackedPhysicalMemory(t)
	phase2Init()

	address := tinygoRuntimeAlloc(32)
	for i := uintptr(0); i < 32; i++ {
		*(*byte)(unsafe.Pointer(address + i)) = 0xA5
	}
	if !tinygoRuntimeFree(address) {
		t.Fatalf("tinygoRuntimeFree should release allocation")
	}

	address2 := tinygoRuntimeAlloc(32)
	if address2 != address {
		t.Fatalf("expected allocator reuse for this test: got 0x%x want 0x%x", address2, address)
	}
	for i := uintptr(0); i < 32; i++ {
		if *(*byte)(unsafe.Pointer(address2+i)) != 0 {
			t.Fatalf("expected zeroed byte at offset %d", i)
		}
	}
	_ = ram
}

func TestRuntimeHeapAllocBytesTracksPageSpan(t *testing.T) {
	resetPhase2ForTest()
	ram := configureHostBackedPhysicalMemory(t)
	phase2Init()

	address, ok := runtimeHeapAlloc(pageSizeBytes + 1)
	if !ok {
		t.Fatalf("runtimeHeapAlloc should succeed")
	}
	bytes, tracked := runtimeHeapAllocBytes(address)
	if !tracked {
		t.Fatalf("runtimeHeapAllocBytes should track active allocation")
	}
	if bytes != 2*pageSizeBytes {
		t.Fatalf("tracked bytes = %d, want %d", bytes, 2*pageSizeBytes)
	}
	_ = ram
}

func TestTinygoRuntimeReallocPreservesPrefix(t *testing.T) {
	resetPhase2ForTest()
	ram := configureHostBackedPhysicalMemory(t)
	phase2Init()

	address := tinygoRuntimeAlloc(16)
	for i := uintptr(0); i < 16; i++ {
		*(*byte)(unsafe.Pointer(address + i)) = byte(i + 1)
	}

	resized := tinygoRuntimeRealloc(address, 48)
	if resized == 0 {
		t.Fatalf("tinygoRuntimeRealloc should return a new allocation")
	}
	for i := uintptr(0); i < 16; i++ {
		if *(*byte)(unsafe.Pointer(resized+i)) != byte(i+1) {
			t.Fatalf("byte %d was not preserved", i)
		}
	}
	_ = ram
}
