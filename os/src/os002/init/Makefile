main.ll: main.c
	clang -emit-llvm -S main.c -o main.ll
main.bc: main.ll
	llvm-as main.ll -o main.bc
main.s: main.bc
	llc main.bc -o main.s -march=x86
main.ll.asm: main.bc
	llc main.bc -o main.ll.asm -march=x86 --x86-asm-syntax=intel
main.o: main.s
	as main.s -o main.o --32
#main.o: main.bc
#	llc main.bc -o main.o -filetype=obj -march=x86

func.o: func.asm
	nasm func.asm -o func.o -l func.lst -f coff

init.bin: main.o func.o
	# ld func.o main.o -o main.bin -m elf_i386
	# 支持的仿真： elf_x86_64 elf32_x86_64 elf_i386 elf_iamcu elf_l1om elf_k1om i386pep i386pe
	# main.o -> elf32-i386
	# func.o -> pe-i386
	ld func.o main.o -o main.bin -m i386pe

m:
	sudo mount -o loop ../os.img ../../point
um:
	sudo umount ../../point
wcp: init.bin
	sudo cp ./init.bin ../../point/
w: m wcp um

clean:
	rm *.ll *.bc *.s *.o *.bin *.lst *.ll.asm
