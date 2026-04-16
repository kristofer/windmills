package main

import "testing"

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
