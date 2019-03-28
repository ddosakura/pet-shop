%include 'config.inc'
%include 'pm.inc'
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

;jmp		go_to_protect

;mov     al, 0x13           ; 显示器模式
;mov     ah, 0x00           ; BIOS中断-设置显示器模式
mov     ax, 0x0013
int     0x10                ; BIOS中断

mov     BYTE    [VMODE], 8  ; 记录画面模式
mov     WORD    [SCRNX], 320
mov     WORD    [SCRNY], 200
mov     DWORD   [VRAM], 0x000a0000
; 取得键盘LED指示灯状态
mov     ah, 0x02
int     0x16
mov     [LEDS], al

go_to_protect:
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
finx:
LoopHLT finx



; 32 位代码段. 由实模式跳入.
[BITS	32]
LABEL_SEG_CODE32:
	jmp test_videoX

	mov	ax, SelectorData
	mov	ds, ax			; 数据段选择子
	mov	ax, SelectorTest
	mov	es, ax			; 测试段选择子
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
;;.1:
;;	lodsb
;;	test	al, al
;;	jz	.2
;;	mov	[gs:edi], ax
;;	add	edi, 2
;;	jmp	.1
;;.2:	; 显示完毕
	call	DispReturn
	call	TestRead
	call	TestWrite
	call	TestRead
	; 到此停止
	jmp	SelectorCode16:0
TestRead:
	xor	esi, esi
	mov	ecx, 8
.loop:
	mov	al, [es:esi]
	call	DispAL
	inc	esi
	loop	.loop
	call	DispReturn
	ret
TestWrite:
	push	esi
	push	edi
	xor	esi, esi
	xor	edi, edi
	mov	esi, OffsetStrTest	; 源数据偏移
	cld
.1:
	lodsb
	test	al, al
	jz	.2
	mov	[es:edi], al
	inc	edi
	jmp	.1
.2:
	pop	edi
	pop	esi
	ret
DispAL:
	push	ecx
	push	edx
	mov	ah, 0Ch			; 0000: 黑底    1100: 红字
	mov	dl, al
	shr	al, 4
	mov	ecx, 2
.begin:
	and	al, 01111b
	cmp	al, 9
	ja	.1
	add	al, '0'
	jmp	.2
.1:
	sub	al, 0Ah
	add	al, 'A'
.2:
	mov	[gs:edi], ax
	add	edi, 2
	mov	al, dl
	loop	.begin
	add	edi, 2
	pop	edx
	pop	ecx
	ret
DispReturn:
	push	eax
	push	ebx
	mov	eax, edi
	mov	bl, 160
	div	bl
	and	eax, 0FFh
	inc	eax
	mov	bl, 160
	mul	bl
	mov	edi, eax
	pop	ebx
	pop	eax
	ret

test_videoX:
mov		ax, SelectorVideoX
mov		gs, ax
mov		edi, 0x0
mov		al, 0xf
print_loop:
mov 	[gs:edi], al
inc		edi
cmp		edi, 0x10000
jne		print_loop
fin:
hlt
jmp	fin

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
and		al, 11111110b
mov		cr0, eax
LABEL_GO_BACK_TO_REAL:
; 段地址会在程序开始处被设置成正确的值
jmp		0:LABEL_REAL_ENTRY
Code16Len	equ	$-LABEL_SEG_CODE16



%include	"gdt.inc"
