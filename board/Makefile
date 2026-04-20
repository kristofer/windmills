TINYGO ?= tinygo
TARGET ?= ../targets/esp32s3-windmills.json
ESPTOOL ?= esptool
PORT ?= /dev/ttyACM0 # /dev/ttyAMA0
BAUD ?= 115200

FLASH_FLAGS = -target $(TARGET) -scheduler=none -gc=none -panic=trap -opt=2 -port $(PORT)

.PHONY: test01 test02 test03 test04 test05 monitor help

test01:
	$(TINYGO) flash $(FLASH_FLAGS) ./test01_blink

test02:
	$(TINYGO) flash $(FLASH_FLAGS) ./test02_uart

test03:
	$(TINYGO) flash $(FLASH_FLAGS) ./test03_usb_serial

test04:
	$(TINYGO) flash $(FLASH_FLAGS) ./test04_usb_serial_nodelay

test05:
	$(TINYGO) flash $(FLASH_FLAGS) ./test05_combined

monitor:
	python3 -m serial.tools.miniterm $(PORT) $(BAUD)

help:
	@echo "Board test targets:"
	@echo "  test01  - Blink (empty loop, tests flash+boot)"
	@echo "  test02  - UART0 println test"
	@echo "  test03  - USB-Serial/JTAG direct register write"
	@echo "  test04  - USB-Serial/JTAG no initial delay"
	@echo "  test05  - Dual output (UART0 + USB-Serial/JTAG)"
	@echo "  monitor - Open miniterm on $(PORT) at $(BAUD)"
