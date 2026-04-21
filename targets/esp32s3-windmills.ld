/* Linker script for ESP32-S3 (Xtensa LX7, Harvard architecture)
 *
 * The ESP32-S3 has separate instruction and data buses that map to the
 * same physical 416 KB of internal SRAM through different address windows:
 *
 *   IRAM  0x40370000 – 0x403DFFFF  (instruction bus, execute)
 *   DRAM  0x3FC88000 – 0x3FCFFFFF  (data bus, read/write)
 *
 * Code placed at IRAM addresses is not accessible as data (and vice versa),
 * so string literals and other read-only data MUST go into DRAM, not IRAM.
 *
 * The boot ROM loads the image through the data bus, so all loadable
 * sections need a Load Memory Address (LMA) in DRAM even when their
 * Virtual Memory Address (VMA) is in IRAM.  This is expressed with the
 * linker directive  "> IRAM AT > DRAM".
 *
 * Additionally, flash-mapped regions are available for XIP:
 *
 *   IROM  0x42000000  (instruction bus, up to 32 MB)
 *   DROM  0x3C000000  (data bus, up to 32 MB)
 *
 * These are not used yet but reserved for future execute-in-place support.
 */

MEMORY
{
    /* Internal SRAM 1 — data bus window */
    DRAM (rw)  : ORIGIN = 0x3FC88000, LENGTH = 416K

    /* Internal SRAM 1 — instruction bus window (same physical memory) */
    IRAM (x)   : ORIGIN = 0x40370000, LENGTH = 416K

    /* Flash-mapped regions (reserved for future XIP use) */
    DROM (r)   : ORIGIN = 0x3C000000, LENGTH = 32M
    IROM (rx)  : ORIGIN = 0x42000000, LENGTH = 32M
}

/* TinyGo runtime entry point (provided by src/device/esp/esp32.S) */
ENTRY(call_start_cpu0)

SECTIONS
{
    /* -----------------------------------------------------------------
     * Stack
     *
     * Placed at the very bottom of DRAM so that a stack overflow will
     * fault instead of silently corrupting the heap / globals.
     * ----------------------------------------------------------------- */
    .stack (NOLOAD) :
    {
        . = ALIGN(16);
        . += _stack_size;
        _stack_top = .;
    } > DRAM

    /* -----------------------------------------------------------------
     * Startup code
     *
     * call_start_cpu0 must be the first code the bootloader jumps to.
     * Xtensa l32r literals must appear *before* the code that
     * references them, so .literal comes first.
     *
     * VMA in IRAM (execute), LMA in DRAM (loaded by boot ROM).
     * ----------------------------------------------------------------- */
    .text.call_start_cpu0 : ALIGN(4)
    {
        *(.literal.call_start_cpu0)
        *(.text.call_start_cpu0)
    } > IRAM AT > DRAM

    /* -----------------------------------------------------------------
     * All other code and literals
     * ----------------------------------------------------------------- */
    .text : ALIGN(4)
    {
        *(.literal .text)
        *(.literal.* .text.*)
        *(.text)
        *(.text.*)
    } > IRAM AT > DRAM

    /* -----------------------------------------------------------------
     * Read-only data (string literals, const tables, etc.)
     *
     * MUST live in DRAM — the data bus cannot read from IRAM addresses.
     * Eventually these could move to DROM (flash) once XIP is enabled,
     * but keeping them in DRAM guarantees access from any context
     * including interrupts.
     * ----------------------------------------------------------------- */
    .rodata : ALIGN(4)
    {
        *(.rodata)
        *(.rodata.*)
    } > DRAM

    /* -----------------------------------------------------------------
     * Mutable initialised data
     * ----------------------------------------------------------------- */
    .data : ALIGN(4)
    {
        _sdata = ABSOLUTE(.);
        *(.data)
        *(.data.*)
        _edata = ABSOLUTE(.);
    } > DRAM

    /* Sanity check: the APP CPU boot ROM uses a stack whose sentinel
     * is at 0x3FFE1320.  Make sure our initialised data does not
     * collide with it.  (Value taken from the boot ROM ELF.) */
    ASSERT(_edata < 0x3FFE1320,
        ".data overlaps with the boot ROM APP-CPU stack — move data or reduce its size")

    /* -----------------------------------------------------------------
     * Zero-initialised data
     *
     * Cleared by clearbss() in the TinyGo runtime before main().
     * ----------------------------------------------------------------- */
    .bss (NOLOAD) : ALIGN(4)
    {
        . = ALIGN(4);
        _sbss = ABSOLUTE(.);
        *(.bss)
        *(.bss.*)
        *(COMMON)
        . = ALIGN(4);
        _ebss = ABSOLUTE(.);
    } > DRAM
}

/* =====================================================================
 * Symbols consumed by the TinyGo runtime
 * ===================================================================== */

/* GC root scanning range */
_globals_start = _sdata;
_globals_end   = _ebss;

/* Heap occupies everything between the end of BSS and top of DRAM */
_heap_start = _ebss;
_heap_end   = ORIGIN(DRAM) + LENGTH(DRAM);

/* Default stack size (16 K to accommodate large kernel structs such as
 * vmAddressSpace in the process table during schedulerInit). */
_stack_size = 16K;

/* =====================================================================
 * ROM function stubs
 *
 * LLVM may emit calls to these during code generation (memcpy for
 * struct copies, memset for zeroing, etc.).  The addresses point to
 * implementations baked into the ESP32-S3 mask ROM.
 *
 * Note: these are the same addresses used by TinyGo's stock esp32s3.ld
 * (inherited from the ESP32 mask ROM — the S3 ROM is layout-compatible
 * at these entry points).
 *
 * Source: ESP-IDF components/esp_rom/esp32/ld/esp32.rom.newlib-funcs.ld
 * ===================================================================== */
memset  = 0x400011e8;
memcpy  = 0x400011f4;
memmove = 0x40001200;
memcmp  = 0x4000120c;

/* =====================================================================
 * libgcc helpers (ROM)
 *
 * LLVM's Xtensa back-end may emit calls to these for operations that
 * have no single-instruction equivalent (64-bit shifts, soft-float,
 * division, etc.).  Addresses are from the ESP32-S3 mask ROM.
 *
 * Note: same addresses as TinyGo's stock esp32s3.ld (inherited from
 * the ESP32 mask ROM table).
 *
 * Source: ESP-IDF components/esp_rom/esp32/ld/esp32.rom.libgcc.ld
 * ===================================================================== */
__absvdi2      = 0x4000216c;
__absvsi2      = 0x40002178;
__adddf3       = 0x40002184;
__addsf3       = 0x40002190;
__addvdi3      = 0x4000219c;
__addvsi3      = 0x400021a8;
__ashldi3      = 0x400021b4;
__ashrdi3      = 0x400021c0;
__bswapdi2     = 0x400021cc;
__bswapsi2     = 0x400021d8;
__clear_cache  = 0x400021e4;
__clrsbdi2     = 0x400021f0;
__clrsbsi2     = 0x400021fc;
__clzdi2       = 0x40002208;
__clzsi2       = 0x40002214;
__cmpdi2       = 0x40002220;
__ctzdi2       = 0x4000222c;
__ctzsi2       = 0x40002238;
__divdc3       = 0x40002244;
__divdf3       = 0x40002250;
__divdi3       = 0x4000225c;
__divsc3       = 0x40002268;
__divsf3       = 0x40002274;
__divsi3       = 0x40002280;
__eqdf2        = 0x4000228c;
__eqsf2        = 0x40002298;
__extendsfdf2  = 0x400022a4;
__ffsdi2       = 0x400022b0;
__ffssi2       = 0x400022bc;
__fixdfdi      = 0x400022c8;
__fixdfsi      = 0x400022d4;
__fixsfdi      = 0x400022e0;
__fixsfsi      = 0x400022ec;
__fixunsdfsi   = 0x400022f8;
__fixunssfdi   = 0x40002304;
__fixunssfsi   = 0x40002310;
__floatdidf    = 0x4000231c;
__floatdisf    = 0x40002328;
__floatsidf    = 0x40002334;
__floatsisf    = 0x40002340;
__floatundidf  = 0x4000234c;
__floatundisf  = 0x40002358;
__floatunsidf  = 0x40002364;
__floatunsisf  = 0x40002370;
__gcc_bcmp     = 0x4000237c;
__gedf2        = 0x40002388;
__gesf2        = 0x40002394;
__gtdf2        = 0x400023a0;
__gtsf2        = 0x400023ac;
__ledf2        = 0x400023b8;
__lesf2        = 0x400023c4;
__lshrdi3      = 0x400023d0;
__ltdf2        = 0x400023dc;
__ltsf2        = 0x400023e8;
__moddi3       = 0x400023f4;
__modsi3       = 0x40002400;
__muldc3       = 0x4000240c;
__muldf3       = 0x40002418;
__muldi3       = 0x40002424;
__mulsc3       = 0x40002430;
__mulsf3       = 0x4000243c;
__mulsi3       = 0x40002448;
__mulvdi3      = 0x40002454;
__mulvsi3      = 0x40002460;
__nedf2        = 0x4000246c;
__negdf2       = 0x40002478;
__negdi2       = 0x40002484;
__negsf2       = 0x40002490;
__negvdi2      = 0x4000249c;
__negvsi2      = 0x400024a8;
__nesf2        = 0x400024b4;
__paritysi2    = 0x400024c0;
__popcountdi2  = 0x400024cc;
__popcountsi2  = 0x400024d8;
__powidf2      = 0x400024e4;
__powisf2      = 0x400024f0;
__subdf3       = 0x400024fc;
__subsf3       = 0x40002508;
__subvdi3      = 0x40002514;
__subvsi3      = 0x40002520;
__truncdfsf2   = 0x4000252c;
__ucmpdi2      = 0x40002538;
__udivdi3      = 0x40002544;
__udivmoddi4   = 0x40002550;
__udivsi3      = 0x4000255c;
__udiv_w_sdiv  = 0x40002568;
__umoddi3      = 0x40002574;
__umodsi3      = 0x40002580;
__unorddf2     = 0x4000258c;
__unordsf2     = 0x40002598;
