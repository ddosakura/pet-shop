build:
	nasm boot.asm -o boot.bin
img:
	bximage
w:
	dd if=boot.bin of=boot.img bs=512 count=1 conv=notrunc
bochs:
	sudo bochs -f boot.bxrc
bochsq:
	sudo bochs -q -f boot.bxrc