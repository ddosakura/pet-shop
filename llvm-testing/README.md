# llvm-testing

## questions

### ld

1. ld: warning: cannot find entry symbol _start

> See: [#1](https://blog.csdn.net/junior1108/article/details/6151906)
```bash
ld -m elf_i386 -dynamic-linker \
    /lib/ld-linux.so.2 \
    -o main.out main.o \
    /usr/lib/crt1.o \
    /usr/lib/crti.o \
    /usr/lib/gcc-lib/i386-redhat-linux/2.96/crtbegin.o \
    -lc \
    /usr/lib/gcc-lib/i386-redhat-linux/2.96/crtend.o \
    /usr/lib/crtn.o
```

> See:[#2](https://blog.csdn.net/rainflood/article/details/75635447)

```bash
ld -m elf_x86_64 -dynamic-linker \
    /lib/x86_64-linux-gnu/ld-linux-x86-64.so.2 \
    -o "main.out" "main.o" \
    /usr/lib/x86_64-linux-gnu/crt1.o \
    /usr/lib/x86_64-linux-gnu/crti.o \
    /usr/lib/gcc/x86_64-linux-gnu/5/crtbegin.o \
    /usr/lib/gcc/x86_64-linux-gnu/5/crtend.o \
    /usr/lib/x86_64-linux-gnu/crtn.o \
    -lc
```

Files:
+ ld-linux.so.2 / ld-linux-x86-64.so.2
+ crt1.o
+ crti.o
+ crtbegin.o
+ crtend.o
+ crtn.o

In my computer:

```bash
ld -m elf_x86_64 -dynamic-linker \
    /lib/ld-linux-x86-64.so.2 \
    -o "main.out" "main.o" \
    /usr/lib/crt1.o \
    /usr/lib/crti.o \
    /usr/lib/gcc/x86_64-pc-linux-gnu/8.2.1/crtbegin.o \
    /usr/lib/gcc/x86_64-pc-linux-gnu/8.2.1/crtend.o \
    /usr/lib/crtn.o \
    -lc
```
