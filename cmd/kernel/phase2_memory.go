package main

const (
	pageSizeBytes            = uintptr(4 * 1024)
	physicalMemorySizeBytes  = uintptr(8 * 1024 * 1024)
	bootAllocatorReserveSize = uintptr(64 * 1024)
	pageFrameCount           = physicalMemorySizeBytes / pageSizeBytes
	pageFrameBitmapBytes     = pageFrameCount / 8
)

type memoryRegionKind uint8

const (
	regionROM memoryRegionKind = iota
	regionIRAM
	regionDRAM
	regionPeripheral
)

type memoryRegion struct {
	start    uintptr
	end      uintptr
	kind     memoryRegionKind
	reserved bool
}

const maxMemoryRegions = 8

var (
	physicalMemoryBase uintptr = 0x3FC80000

	regionTable      [maxMemoryRegions]memoryRegion
	regionTableCount uint8

	bootAllocatorEnabled             bool
	bootAllocatorPermanentlyDisabled bool
	bootAllocatorCursor              uintptr
	bootAllocatorEnd                 uintptr

	pageFrameBitmap [pageFrameBitmapBytes]uint8
)

func addMemoryRegion(start, end uintptr, kind memoryRegionKind, reserved bool) {
	if start >= end {
		panic("memory map: invalid range")
	}
	if int(regionTableCount) >= len(regionTable) {
		panic("memory map: too many regions")
	}
	regionTable[regionTableCount] = memoryRegion{
		start:    start,
		end:      end,
		kind:     kind,
		reserved: reserved,
	}
	regionTableCount++
}

func buildMemoryMap(base uintptr) {
	regionTableCount = 0

	addMemoryRegion(0x40000000, 0x40080000, regionROM, true)
	addMemoryRegion(0x40370000, 0x403E0000, regionIRAM, true)
	addMemoryRegion(0x60000000, 0x61000000, regionPeripheral, true)

	addMemoryRegion(base, base+bootAllocatorReserveSize, regionDRAM, true)
	addMemoryRegion(base+bootAllocatorReserveSize, base+physicalMemorySizeBytes, regionDRAM, false)
}

func addressInRegion(address uintptr, region memoryRegion) bool {
	return address >= region.start && address < region.end
}

func isReservedAddress(address uintptr) bool {
	for i := uint8(0); i < regionTableCount; i++ {
		region := regionTable[i]
		if addressInRegion(address, region) && region.reserved {
			return true
		}
	}
	return false
}

func isUsableDRAMAddress(address uintptr) bool {
	for i := uint8(0); i < regionTableCount; i++ {
		region := regionTable[i]
		if !addressInRegion(address, region) {
			continue
		}
		return region.kind == regionDRAM && !region.reserved
	}
	return false
}

func bootAllocatorInit(base uintptr) {
	if bootAllocatorPermanentlyDisabled {
		panic("boot allocator: permanently disabled")
	}
	bootAllocatorEnabled = true
	bootAllocatorCursor = base
	bootAllocatorEnd = base + bootAllocatorReserveSize
}

func bootAllocatorDisable() {
	bootAllocatorEnabled = false
	bootAllocatorPermanentlyDisabled = true
}

func alignUp(value, alignment uintptr) uintptr {
	mask := alignment - 1
	return (value + mask) &^ mask
}

func bootAlloc(size uintptr) uintptr {
	if !bootAllocatorEnabled {
		panic("boot allocator: disabled")
	}
	size = alignUp(size, pageSizeBytes)
	next := bootAllocatorCursor + size
	if next > bootAllocatorEnd {
		panic("boot allocator: out of reserved memory")
	}
	addr := bootAllocatorCursor
	bootAllocatorCursor = next
	return addr
}

func bitmapSetUsed(frame uint32) {
	byteIndex := frame / 8
	bit := uint8(1 << (frame % 8))
	pageFrameBitmap[byteIndex] |= bit
}

func bitmapSetFree(frame uint32) {
	byteIndex := frame / 8
	bit := uint8(1 << (frame % 8))
	pageFrameBitmap[byteIndex] &^= bit
}

func bitmapIsUsed(frame uint32) bool {
	byteIndex := frame / 8
	bit := uint8(1 << (frame % 8))
	return (pageFrameBitmap[byteIndex] & bit) != 0
}

func pageAllocatorInit(base uintptr) {
	for i := range pageFrameBitmap {
		pageFrameBitmap[i] = 0xFF
	}

	for frame := uint32(0); frame < uint32(pageFrameCount); frame++ {
		address := base + uintptr(frame)*pageSizeBytes
		if isUsableDRAMAddress(address) && !isReservedAddress(address) {
			bitmapSetFree(frame)
		}
	}
}

func allocPage() (uintptr, bool) {
	for frame := uint32(0); frame < uint32(pageFrameCount); frame++ {
		if bitmapIsUsed(frame) {
			continue
		}
		address := physicalMemoryBase + uintptr(frame)*pageSizeBytes
		if !isUsableDRAMAddress(address) || isReservedAddress(address) {
			bitmapSetUsed(frame)
			continue
		}
		bitmapSetUsed(frame)
		return address, true
	}
	return 0, false
}

func freePage(address uintptr) bool {
	if address < physicalMemoryBase || address >= physicalMemoryBase+physicalMemorySizeBytes {
		return false
	}
	if address%pageSizeBytes != 0 {
		return false
	}
	if !isUsableDRAMAddress(address) || isReservedAddress(address) {
		return false
	}
	frame := uint32((address - physicalMemoryBase) / pageSizeBytes)
	if !bitmapIsUsed(frame) {
		return false
	}
	bitmapSetFree(frame)
	return true
}

func phase2Init() {
	buildMemoryMap(physicalMemoryBase)
	bootAllocatorInit(physicalMemoryBase)
	pageAllocatorInit(physicalMemoryBase)
	bootAllocatorDisable()
}
