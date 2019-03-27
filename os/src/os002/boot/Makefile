boot.bin: boot.asm
	nasm boot.asm -o boot.bin -l boot.lst
head.bin: head.asm
	nasm head.asm -o head.bin -l head.lst

# ipl
w: boot.bin
	dd if=boot.bin of=../os.img bs=512 count=1 conv=notrunc
# payload
m:
	sudo mount -o loop ../os.img ../../point
um:
	sudo umount ../../point
wcp: head.bin
	sudo cp ./head.bin ../../point/
wos: m wcp um
wa: w wos

clean:
	rm *.lst *.bin

