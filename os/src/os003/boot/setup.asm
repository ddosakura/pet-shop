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

mov		[_wSPValueInRealMode], sp

; 得到内存数
mov	ebx, 0
mov	di, _MemChkBuf
.loop:
	mov	eax, 0E820h
	mov	ecx, 20
	mov	edx, 0534D4150h
	int	15h
	jc	LABEL_MEM_CHK_FAIL
	add	di, 20
	inc	dword [_dwMCRNumber]
	cmp	ebx, 0
	jne	.loop
	jmp	LABEL_MEM_CHK_OK
LABEL_MEM_CHK_FAIL:
	mov	dword [_dwMCRNumber], 0
LABEL_MEM_CHK_OK:

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
mov		sp, [_wSPValueInRealMode]
DisableA20
LoopHLT



; 32 位代码段. 由实模式跳入.
[BITS	32]
LABEL_SEG_CODE32:
	mov	ax, SelectorData
	mov	ds, ax			; 数据段选择子
	mov	ax, SelectorData
	mov	es, ax
	mov	ax, SelectorVideo
	mov	gs, ax			; 视频段选择子
	mov	ax, SelectorStack
	mov	ss, ax			; 堆栈段选择子
	mov	esp, TopOfStack

	; 下面显示一个字符串
	push	szPMMessage
	call	DispStr
	add	esp, 4

	push	szMemChkTitle
	call	DispStr
	add	esp, 4

	call	DispMemSize		; 显示内存信息
	call	SetupPaging		; 启动分页机制

	; 到此停止
	jmp	SelectorCode16:0

SetupPaging:
	; 根据内存大小计算应初始化多少PDE以及多少页表
	xor	edx, edx
	mov	eax, [dwMemSize]
	mov	ebx, 400000h	; 400000h = 4M = 4096 * 1024, 一个页表对应的内存大小
	div	ebx
	mov	ecx, eax	; 此时 ecx 为页表的个数，也即 PDE 应该的个数
	test	edx, edx
	jz	.no_remainder
	inc	ecx		; 如果余数不为 0 就需增加一个页表
.no_remainder:
	push	ecx		; 暂存页表个数

	; 为简化处理, 所有线性地址对应相等的物理地址. 并且不考虑内存空洞.

	; 首先初始化页目录
	mov	ax, SelectorPageDir	; 此段首地址为 PageDirBase
	mov	es, ax
	xor	edi, edi
	xor	eax, eax
	mov	eax, PageTblBase | PG_P  | PG_USU | PG_RWW
.1:
	stosd
	add	eax, 4096		; 为了简化, 所有页表在内存中是连续的.
	loop	.1

	; 再初始化所有页表
	mov	ax, SelectorPageTbl	; 此段首地址为 PageTblBase
	mov	es, ax
	pop	eax			; 页表个数
	mov	ebx, 1024		; 每个页表 1024 个 PTE
	mul	ebx
	mov	ecx, eax		; PTE个数 = 页表个数 * 1024
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

DispMemSize:
	push	esi
	push	edi
	push	ecx

	mov	esi, MemChkBuf
	mov	ecx, [dwMCRNumber];for(int i=0;i<[MCRNumber];i++)//每次得到一个ARDS
.loop:				  ;{
	mov	edx, 5		  ;  for(int j=0;j<5;j++) //每次得到一个ARDS中的成员
	mov	edi, ARDStruct	  ;  {//依次显示BaseAddrLow,BaseAddrHigh,LengthLow,
.1:				  ;             LengthHigh,Type
	push	dword [esi]	  ;
	call	DispInt		  ;    DispInt(MemChkBuf[j*4]); //显示一个成员
	pop	eax		  ;
	stosd			  ;    ARDStruct[j*4] = MemChkBuf[j*4];
	add	esi, 4		  ;
	dec	edx		  ;
	cmp	edx, 0		  ;
	jnz	.1		  ;  }
	call	DispReturn	  ;  printf("\n");
	cmp	dword [dwType], 1 ;  if(Type == AddressRangeMemory)
	jne	.2		  ;  {
	mov	eax, [dwBaseAddrLow];
	add	eax, [dwLengthLow];
	cmp	eax, [dwMemSize]  ;    if(BaseAddrLow + LengthLow > MemSize)
	jb	.2		  ;
	mov	[dwMemSize], eax  ;    MemSize = BaseAddrLow + LengthLow;
.2:				  ;  }
	loop	.loop		  ;}
				  ;
	call	DispReturn	  ;printf("\n");
	push	szRAMSize	  ;
	call	DispStr		  ;printf("RAM size:");
	add	esp, 4		  ;
				  ;
	push	dword [dwMemSize] ;
	call	DispInt		  ;DispInt(MemSize);
	add	esp, 4		  ;

	pop	ecx
	pop	edi
	pop	esi
	ret

; 库函数
%include	"lib.inc"

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
