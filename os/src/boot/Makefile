build:
	nasm boot.asm -o boot.bin -l boot.lst
buildos:
	nasm os.asm -o os.bin -l os.lst

img:
	bximage
mkfs:
	mkfs.vfat os.img
see1:
	xxd -u -a -g 1 -c 16 -s +0x2600 -l 512 os.img
see2:
	xxd -u -a -g 1 -c 16 -s +0x4200 -l 512 os.img

w:
	dd if=boot.bin of=boot.img bs=512 count=1 conv=notrunc
bochs:
	sudo bochs -f boot.bxrc
bochsq:
	sudo bochs -q -f boot.bxrc

#read:
#	readelf -s boot.bin
#dump:
#	objdump -d boot.bin
#gdb:
#