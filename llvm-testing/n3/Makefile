main.ll: main.go
	go run main.go
main.bc: main.ll
	llvm-as main.ll -o main.bc
run:
	lli main.bc
dis:
	llvm-dis main.bc -o main.ll
main.s: main.bc
	llc main.bc -o main.s
build: main.s
	# as main.s -o main.out; chmod +x main.out
	# as main.s -o main.o; ld -o main.out main.o; chmod +x main.out
	as main.s -o main.o; ld -m elf_x86_64 -dynamic-linker \
    /lib/ld-linux-x86-64.so.2 \
    -o "main.out" "main.o" \
    /usr/lib/crt1.o \
    /usr/lib/crti.o \
    /usr/lib/gcc/x86_64-pc-linux-gnu/8.2.1/crtbegin.o \
    /usr/lib/gcc/x86_64-pc-linux-gnu/8.2.1/crtend.o \
    /usr/lib/crtn.o \
    -lc; chmod +x main.out

# See: https://blog.csdn.net/softee/article/details/41128667

hex:
	hexdump main.out
	# od -h main.out


main.o: main.bc
	llc main.bc -o main.o -filetype=obj
main.out: main.o
	ld -m elf_x86_64 -dynamic-linker \
    /lib/ld-linux-x86-64.so.2 \
    -o "main.out" "main.o" \
    /usr/lib/crt1.o \
    /usr/lib/crti.o \
    /usr/lib/gcc/x86_64-pc-linux-gnu/8.2.1/crtbegin.o \
    /usr/lib/gcc/x86_64-pc-linux-gnu/8.2.1/crtend.o \
    /usr/lib/crtn.o \
    -lc; chmod +x main.out
# llc32:
# 	llc main.bc -o main.x86.s -mtriple=x86

clean:
	rm *.bc *.ll *.o *.out *.s *.asm

debug:
	llc main.bc -o main.asm --x86-asm-syntax=intel
