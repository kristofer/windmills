package main

import "testing"

func resetPhase2ForTest() {
	physicalMemoryBase = 0x20000000
	regionTableCount = 0
	bootAllocatorEnabled = false
	bootAllocatorPermanentlyDisabled = false
	bootAllocatorCursor = 0
	bootAllocatorEnd = 0
	nextFreeFrame = 0
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

	wantPages := int((physicalMemorySizeBytes - bootAllocatorReserveSize) / pageSizeBytes)
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
	want := physicalMemoryBase + bootAllocatorReserveSize
	if address != want {
		t.Fatalf("first alloc page = 0x%x, want 0x%x", address, want)
	}
}
