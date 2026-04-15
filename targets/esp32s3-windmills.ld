ENTRY(_start)

MEMORY
{
  iram (rx)  : ORIGIN = 0x40370000, LENGTH = 0x00080000
  dram (rwx) : ORIGIN = 0x3FC88000, LENGTH = 0x00080000
}

SECTIONS
{
  .text :
  {
    KEEP(*(.text._start))
    *(.text*)
    *(.rodata*)
  } > iram

  .data :
  {
    *(.data*)
  } > dram

  .bss (NOLOAD) :
  {
    *(.bss*)
    *(COMMON)
  } > dram
}
