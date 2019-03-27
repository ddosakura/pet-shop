; 先复制在这，之后处理
; https://blog.csdn.net/ekkie/article/details/51379593
; http://blog.chinaunix.net/uid-28323465-id-3487762.html

; PIC 可编程中断控制器 有两个PIC 每个PIC有8个输入0-7
; cli关闭所有中断，sti打开所有中断
; PIC����؂̊��荞�݂��󂯕t���Ȃ��悤�ɂ���
;	AT�݊��@�̎d�l�ł́APIC�̏�����������Ȃ�A
;	������CLI�O�ɂ���Ă����Ȃ��ƁA���܂Ƀn���O�A�b�v����
;	PIC�̏������͂��Ƃł��

mov     al, 0xff
out     0x21, al                ; pic0的端口
nop                             ; 太快可能会有问题
out		0xa1, al                ; pic1的端口
cli                             ; 关中断

; linux1.0 似乎也用这个方法打开 A20
call    waitkbdout
mov     al, 0xd1
out     0x64, al
call    waitkbdout
mov     al, 0xdf                ; enable A20
out     0x60, al
call    waitkbdout

; 切换到保护模式
lgdt	[GDTR0]			        ; 加载 GDTR
mov     eax, cr0
and     eax, 0x7fffffff         ; 禁止分页
or      eax, 1                  ; 保护模式开关
mov     cr0, eax
jmp     pipelineflush

pipelineflush:
mov     ax, 1*8                 ; 取数据段偏移
mov     ds, ax
mov     es, ax
mov     fs, ax
mov     gs, ax
mov     ss, ax

; 主程序加载到      0x280000
mov     esi, bootpack
mov     edi, BOTPAK
mov     ecx, 512*1024/4
call    memcpy

; boot程序加载到    0x100000
mov     esi, 0x7c00
mov     edi, DSKCAC
mov     ecx, 512/4
call    memcpy

mov     esi, DSKCAC0+512        ; 跳过引导扇区
mov     edi, DSKCAC+512
mov     ecx, 0
mov     cl, BYTE [CYLS]
imul    ecx, 512*18*2/4
sub     ecx, 512/4              ; 扇区数量
call    memcpy

; 跳转主程序
mov     ebx, BOTPAK
mov     ecx, [ebx+16]
add     ecx, 3
shr     ecx, 2
jz      skip
mov     esi, [ebx+20]
add     esi, ebx
mov     edi, [ebx+12]
call    memcpy
skip:
mov     esp, [ebx+12]           ; 设置栈地址
jmp     dword 2*8:0x0000001b    ; 主程序

waitkbdout:
in      al, 0x64
and     al, 0x02                ; cpu可向键盘写命令时为1
jnz     waitkbdout
ret

memcpy:
mov     eax, [esi]
add     esi, 4
mov     [edi], eax
add     edi, 4
sub     ecx, 1
jnz     memcpy
ret

; 全局变量表
ALIGNB	16                      ; 16字节对齐 bss段
GDT0:
resb    8                       ; 第一项规定为0
dw      0xffff, 0, 0x9200, 0x00cf	; 数据段
dw      0xffff, 0, 0x9a28, 0x0047	; 程序段

dw      0

GDTR0:
dw      8*3-1                   ; 表的大小(字节)减1
dd      GDT0                    ; 表的地址

ALIGNB	16
bootpack:
