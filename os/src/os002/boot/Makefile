boot.bin: boot.asm
	nasm boot.asm -o boot.bin -l boot.lst
setup.bin: setup.asm
	nasm setup.asm -o setup.bin -l setup.lst

# ipl
boot: boot.bin
	dd if=boot.bin of=../os.img bs=512 count=1 conv=notrunc

# payload
m:
	sudo mount -o loop ../os.img ../../point
um:
	sudo umount ../../point
wcp: setup.bin
	sudo cp ./setup.bin ../../point/
w: setup.bin m wcp um

all: boot w

clean:
	rm *.lst *.bin
