# ELF

## 编译连接

32bit:
```bash
nasm -f elf32 hello.asm -o hello32.o
ld hello32.o -s -o hello32.bin -m elf_i386
```

64bit:
```bash
nasm -f elf64 hello.asm -o hello64.o
ld hello64.o -s -o hello64.bin
```
