%include 'config.inc'
%include 'pm.inc'
%include 'macro.inc'
offset	equ 0
org		offset

[BITS	16]
mov		ax, cs
mov		ds, ax
mov		es, ax
mov		ss, ax
mov		sp, offset
; 设置回跳段地址
mov		[LABEL_GO_BACK_TO_REAL+3], ax

; 进入保护模式
; 1. 准备 GDT (Global Descriptor Table 全局描述表)
;    用 lgdt指令 加载 gdtr寄存器
; 2. 关中断
;    打开 A20 （有多种方法）
; 3. 置 cr0 的 PE 位为 1
; 4. jmp

mov		[SPValueInRealMode], sp

; 初始化 16 位代码段描述符
InitGDT LABEL_SEG_CODE16, LABEL_DESC_CODE16
; 初始化 32 位代码段描述符
InitGDT LABEL_SEG_CODE32, LABEL_DESC_CODE32

; 初始化数据段描述符
InitGDT LABEL_DATA, LABEL_DESC_DATA
; 初始化堆栈段描述符
InitGDT LABEL_STACK, LABEL_DESC_STACK

; 为加载 GDTR 作准备 & 加载 GDTR & 关中断 & 打开 A20
LoadGDT LABEL_GDT, GdtPtr
EnableA20

; 准备切换到保护模式
mov		eax, cr0
;and     eax, 0x7fffffff         ; 禁止分页(貌似默认就是禁止的)
or		eax, 1					; 保护模式开关
mov		cr0, eax

; 进入保护模式
jmp	dword SelectorCode32:0

; 从保护模式跳回到实模式就到了这里
LABEL_REAL_ENTRY:
mov		ax, cs
mov		ds, ax
mov		es, ax
mov		ss, ax
mov		sp, [SPValueInRealMode]
DisableA20
LoopHLT



; 32 位代码段. 由实模式跳入.
[BITS	32]
LABEL_SEG_CODE32:
	call SetupPaging

	mov	ax, SelectorData
	mov	ds, ax			; 数据段选择子
	mov	ax, SelectorVideo
	mov	gs, ax			; 视频段选择子
	mov	ax, SelectorStack
	mov	ss, ax			; 堆栈段选择子
	mov	esp, TopOfStack

	; 下面显示一个字符串
	mov	ah, 0Ch			; 0000: 黑底    1100: 红字
	xor	esi, esi
	xor	edi, edi
	mov	esi, OffsetPMMessage	; 源数据偏移
	mov	edi, (80 * 10 + 0) * 2	; 目的数据偏移。屏幕第 10 行, 第 0 列。
	cld
.1:
	lodsb
	test	al, al
	jz	.2
	mov	[gs:edi], ax
	add	edi, 2
	jmp	.1
.2:	; 显示完毕

	; 到此停止
	jmp	SelectorCode16:0

SetupPaging:
	; 为简化处理, 所有线性地址对应相等的物理地址.

	; 首先初始化页目录
	mov	ax, SelectorPageDir	; 此段首地址为 PageDirBase
	mov	es, ax
	mov	ecx, 1024		; 共 1K 个表项
	xor	edi, edi
	xor	eax, eax
	mov	eax, PageTblBase | PG_P  | PG_USU | PG_RWW
.1:
	stosd
	add	eax, 4096		; 为了简化, 所有页表在内存中是连续的.
	loop	.1

	; 再初始化所有页表 (1K 个, 4M 内存空间)
	mov	ax, SelectorPageTbl	; 此段首地址为 PageTblBase
	mov	es, ax
	mov	ecx, 1024 * 1024	; 共 1M 个页表项, 也即有 1M 个页
	xor	edi, edi
	xor	eax, eax
	mov	eax, PG_P  | PG_USU | PG_RWW
.2:
	stosd
	add	eax, 4096		; 每一页指向 4K 的空间
	loop	.2

	mov	eax, PageDirBase
	mov	cr3, eax
	mov	eax, cr0
	or	eax, 80000000h
	mov	cr0, eax
	jmp	short .3
.3:
	nop

	ret

SegCode32Len	equ	$ - LABEL_SEG_CODE32



; 16 位代码段. 由 32 位代码段跳入, 跳出后到实模式
ALIGN	32
[BITS	16]
LABEL_SEG_CODE16:
; 跳回实模式:
mov		ax, SelectorNormal
mov		ds, ax
mov		es, ax
mov		fs, ax
mov		gs, ax
mov		ss, ax

mov		eax, cr0
;and		al, 11111110b
and		eax, 0x7FFFFFFE			; PE=0, PG=0
mov		cr0, eax
LABEL_GO_BACK_TO_REAL:
; 段地址会在程序开始处被设置成正确的值
jmp		0:LABEL_REAL_ENTRY
Code16Len	equ	$-LABEL_SEG_CODE16



%include	"gdt.inc"
%include	"page.inc"
