package main

import "unsafe"

const (
	pageSizeBytes                   = uintptr(4 * 1024)
	physicalMemorySizeBytes         = uintptr(8 * 1024 * 1024)
	bootAllocatorReserveSize        = uintptr(64 * 1024)
	pageFrameCount           uint32 = uint32(physicalMemorySizeBytes / pageSizeBytes)
	pageFrameBitmapBytes            = pageFrameCount / 8

	// ESP32-S3 fixed memory-mapped hardware regions.
	romStart        = uintptr(0x40000000)
	romEnd          = uintptr(0x40080000)
	iramStart       = uintptr(0x40370000)
	iramEnd         = uintptr(0x403E0000)
	peripheralStart = uintptr(0x60000000)
	peripheralEnd   = uintptr(0x61000000)

	// ESP32-S3 DRAM base used for host-model defaults; this value remains configurable.
	defaultPhysicalMemoryBase = uintptr(0x3FC80000)
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
const maxRuntimeHeapAllocs = 128

var (
	physicalMemoryBase uintptr = defaultPhysicalMemoryBase

	regionTable      [maxMemoryRegions]memoryRegion
	regionTableCount uint8

	mem_init_complete bool

	bootAllocatorEnabled             bool
	bootAllocatorPermanentlyDisabled bool
	bootAllocatorCursor              uintptr
	bootAllocatorEnd                 uintptr

	pageFrameBitmap   [pageFrameBitmapBytes]uint8
	nextFreeFrame     uint32
	userNextFreeFrame uint32

	kernelPoolStartFrame uint32
	kernelPoolEndFrame   uint32
	userPoolStartFrame   uint32
	userPoolEndFrame     uint32
)

type allocatorPool uint8

const (
	kernelMemoryPool allocatorPool = iota
	userMemoryPool
)

type runtimeHeapAllocation struct {
	base  uintptr
	pages uintptr
	inUse bool
}

var runtimeHeapAllocs [maxRuntimeHeapAllocs]runtimeHeapAllocation

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

	addMemoryRegion(romStart, romEnd, regionROM, true)
	addMemoryRegion(iramStart, iramEnd, regionIRAM, true)
	addMemoryRegion(peripheralStart, peripheralEnd, regionPeripheral, true)

	addMemoryRegion(base, base+bootAllocatorReserveSize, regionDRAM, true)
	addMemoryRegion(base+bootAllocatorReserveSize, base+physicalMemorySizeBytes, regionDRAM, false)
}

func addressInRegion(address uintptr, region memoryRegion) bool {
	return address >= region.start && address < region.end
}

func findMemoryRegion(address uintptr) (memoryRegion, bool) {
	for i := uint8(0); i < regionTableCount; i++ {
		region := regionTable[i]
		if addressInRegion(address, region) {
			return region, true
		}
	}
	return memoryRegion{}, false
}

func isReservedAddress(address uintptr) bool {
	region, ok := findMemoryRegion(address)
	return ok && region.reserved
}

func isUsableDRAMAddress(address uintptr) bool {
	region, ok := findMemoryRegion(address)
	return ok && region.kind == regionDRAM && !region.reserved
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
	if alignment == 0 || (alignment&(alignment-1)) != 0 {
		panic("alignUp: alignment must be power of two")
	}
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
	nextFreeFrame = 0

	for frame := uint32(0); frame < pageFrameCount; frame++ {
		address := base + uintptr(frame)*pageSizeBytes
		if isUsableDRAMAddress(address) {
			bitmapSetFree(frame)
		}
	}
}

func initMemoryPools() {
	usableStartFrame := uint32(bootAllocatorReserveSize / pageSizeBytes)
	usableFrameCount := pageFrameCount - usableStartFrame
	kernelFrameCount := usableFrameCount / 2
	if kernelFrameCount == 0 && usableFrameCount > 0 {
		kernelFrameCount = usableFrameCount
	}

	kernelPoolStartFrame = usableStartFrame
	kernelPoolEndFrame = usableStartFrame + kernelFrameCount
	userPoolStartFrame = kernelPoolEndFrame
	userPoolEndFrame = pageFrameCount
	nextFreeFrame = kernelPoolStartFrame
	userNextFreeFrame = userPoolStartFrame
}

func poolBounds(pool allocatorPool) (start, end uint32, ok bool) {
	switch pool {
	case kernelMemoryPool:
		return kernelPoolStartFrame, kernelPoolEndFrame, kernelPoolStartFrame < kernelPoolEndFrame
	case userMemoryPool:
		return userPoolStartFrame, userPoolEndFrame, userPoolStartFrame < userPoolEndFrame
	default:
		return 0, 0, false
	}
}

func poolNextFreeFrame(pool allocatorPool) uint32 {
	if pool == userMemoryPool {
		return userNextFreeFrame
	}
	return nextFreeFrame
}

func setPoolNextFreeFrame(pool allocatorPool, frame uint32) {
	if pool == userMemoryPool {
		userNextFreeFrame = frame
		return
	}
	nextFreeFrame = frame
}

func frameFromAddress(address uintptr) (uint32, bool) {
	if address < physicalMemoryBase || address >= physicalMemoryBase+physicalMemorySizeBytes {
		return 0, false
	}
	if address%pageSizeBytes != 0 {
		return 0, false
	}
	return uint32((address - physicalMemoryBase) / pageSizeBytes), true
}

func allocPageFromPool(pool allocatorPool) (uintptr, bool) {
	start, end, ok := poolBounds(pool)
	if !ok {
		return 0, false
	}

	span := end - start
	baseFrame := poolNextFreeFrame(pool)
	if baseFrame < start || baseFrame >= end {
		baseFrame = start
	}

	for scanned := uint32(0); scanned < span; scanned++ {
		frame := start + ((baseFrame - start + scanned) % span)
		if bitmapIsUsed(frame) {
			continue
		}
		address := physicalMemoryBase + uintptr(frame)*pageSizeBytes
		if !isUsableDRAMAddress(address) {
			bitmapSetUsed(frame)
			continue
		}
		bitmapSetUsed(frame)
		next := frame + 1
		if next >= end {
			next = start
		}
		setPoolNextFreeFrame(pool, next)
		return address, true
	}
	return 0, false
}

func freePageInPool(address uintptr, pool allocatorPool) bool {
	frame, ok := frameFromAddress(address)
	if !ok || !isUsableDRAMAddress(address) {
		return false
	}
	start, end, poolOK := poolBounds(pool)
	if !poolOK || frame < start || frame >= end {
		return false
	}
	if !bitmapIsUsed(frame) {
		return false
	}
	bitmapSetFree(frame)
	next := poolNextFreeFrame(pool)
	if next < start || next >= end || frame < next {
		setPoolNextFreeFrame(pool, frame)
	}
	return true
}

func allocContigFromPool(pool allocatorPool, pageCount uintptr) (uintptr, bool) {
	if pageCount == 0 {
		return 0, false
	}

	start, end, ok := poolBounds(pool)
	if !ok {
		return 0, false
	}
	span := end - start
	requiredFrames := uint32(pageCount)
	if uintptr(requiredFrames) != pageCount || requiredFrames > span {
		return 0, false
	}

	baseFrame := poolNextFreeFrame(pool)
	if baseFrame < start || baseFrame >= end {
		baseFrame = start
	}

	for scanned := uint32(0); scanned < span; scanned++ {
		candidate := start + ((baseFrame - start + scanned) % span)
		if candidate+requiredFrames > end {
			continue
		}

		fits := true
		for offset := uint32(0); offset < requiredFrames; offset++ {
			frame := candidate + offset
			address := physicalMemoryBase + uintptr(frame)*pageSizeBytes
			if bitmapIsUsed(frame) || !isUsableDRAMAddress(address) {
				fits = false
				break
			}
		}
		if !fits {
			continue
		}

		for offset := uint32(0); offset < requiredFrames; offset++ {
			bitmapSetUsed(candidate + offset)
		}
		next := candidate + requiredFrames
		if next >= end {
			next = start
		}
		setPoolNextFreeFrame(pool, next)
		return physicalMemoryBase + uintptr(candidate)*pageSizeBytes, true
	}

	return 0, false
}

func AllocPage() (uintptr, bool) {
	return allocPageFromPool(kernelMemoryPool)
}

func FreePage(address uintptr) bool {
	return freePageInPool(address, kernelMemoryPool)
}

func AllocContig(pageCount uintptr) (uintptr, bool) {
	return allocContigFromPool(kernelMemoryPool, pageCount)
}

func allocPage() (uintptr, bool) {
	return AllocPage()
}

func freePage(address uintptr) bool {
	return FreePage(address)
}

func runtimeHeapAlloc(size uintptr) (uintptr, bool) {
	if !mem_init_complete || size == 0 {
		return 0, false
	}
	pageCount := alignUp(size, pageSizeBytes) / pageSizeBytes
	address, ok := AllocContig(pageCount)
	if !ok {
		return 0, false
	}
	if !runtimeHeapTrackAlloc(address, pageCount) {
		for i := uintptr(0); i < pageCount; i++ {
			FreePage(address + i*pageSizeBytes)
		}
		return 0, false
	}
	return address, true
}

func runtimeHeapAllocBytes(address uintptr) (uintptr, bool) {
	for i := range runtimeHeapAllocs {
		if !runtimeHeapAllocs[i].inUse || runtimeHeapAllocs[i].base != address {
			continue
		}
		return runtimeHeapAllocs[i].pages * pageSizeBytes, true
	}
	return 0, false
}

func runtimeHeapFree(address uintptr) bool {
	if !mem_init_complete {
		return false
	}
	pageCount, ok := runtimeHeapUntrackAlloc(address)
	if !ok {
		return false
	}
	for i := uintptr(0); i < pageCount; i++ {
		if !FreePage(address + i*pageSizeBytes) {
			return false
		}
	}
	return true
}

func tinygoRuntimeAlloc(size uintptr) uintptr {
	if size == 0 {
		return 0
	}
	address, ok := runtimeHeapAlloc(size)
	if !ok {
		panic("tinygo runtime: allocation failed")
	}
	clearMemory(address, alignUp(size, pageSizeBytes))
	return address
}

func tinygoRuntimeFree(address uintptr) bool {
	return runtimeHeapFree(address)
}

func tinygoRuntimeRealloc(address uintptr, size uintptr) uintptr {
	if size == 0 {
		if address != 0 {
			_ = runtimeHeapFree(address)
		}
		return 0
	}
	if address == 0 {
		return tinygoRuntimeAlloc(size)
	}

	oldBytes, ok := runtimeHeapAllocBytes(address)
	if !ok {
		panic("tinygo runtime: realloc invalid address")
	}

	newAddress := tinygoRuntimeAlloc(size)
	copyBytes := oldBytes
	if size < copyBytes {
		copyBytes = size
	}
	copyMemory(newAddress, address, copyBytes)

	if !runtimeHeapFree(address) {
		panic("tinygo runtime: realloc free failed")
	}

	return newAddress
}

func clearMemory(address uintptr, size uintptr) {
	for i := uintptr(0); i < size; i++ {
		*(*byte)(unsafe.Pointer(address + i)) = 0
	}
}

func copyMemory(dst uintptr, src uintptr, size uintptr) {
	if dst == src || size == 0 {
		return
	}
	if dst < src || dst >= src+size {
		for i := uintptr(0); i < size; i++ {
			*(*byte)(unsafe.Pointer(dst + i)) = *(*byte)(unsafe.Pointer(src + i))
		}
		return
	}
	for i := size; i > 0; i-- {
		off := i - 1
		*(*byte)(unsafe.Pointer(dst + off)) = *(*byte)(unsafe.Pointer(src + off))
	}
}

func runtimeHeapTrackAlloc(address uintptr, pageCount uintptr) bool {
	for i := range runtimeHeapAllocs {
		if runtimeHeapAllocs[i].inUse {
			continue
		}
		runtimeHeapAllocs[i] = runtimeHeapAllocation{
			base:  address,
			pages: pageCount,
			inUse: true,
		}
		return true
	}
	return false
}

func runtimeHeapUntrackAlloc(address uintptr) (uintptr, bool) {
	for i := range runtimeHeapAllocs {
		if !runtimeHeapAllocs[i].inUse || runtimeHeapAllocs[i].base != address {
			continue
		}
		pageCount := runtimeHeapAllocs[i].pages
		runtimeHeapAllocs[i] = runtimeHeapAllocation{}
		return pageCount, true
	}
	return 0, false
}

func phase2Init() {
	buildMemoryMap(physicalMemoryBase)
	bootAllocatorInit(physicalMemoryBase)
	pageAllocatorInit(physicalMemoryBase)
	initMemoryPools()
	bootAllocatorDisable()
	mem_init_complete = true
	consoleLogln("phase2init done")
}
