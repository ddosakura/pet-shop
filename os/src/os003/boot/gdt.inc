; GDT
;                              段基址,   段界限,          属性
; 空描述符
LABEL_GDT:          Descriptor 0,		0,              0
; Normal 描述符
LABEL_DESC_NORMAL:  Descriptor 0,       0xffff,         DA_DRW
; 非一致代码段 (32位)
LABEL_DESC_CODE32:  Descriptor 0,       SegCode32Len-1, DA_CR|DA_32
; 非一致代码段 (16位)
LABEL_DESC_CODE16:  Descriptor 0,       0ffffh,         DA_C

; Data数据段
LABEL_DESC_DATA:    Descriptor 0,       DataLen-1,      DA_DRW
; Stack 全局堆栈段 (32位)
LABEL_DESC_STACK:   Descriptor 0,       TopOfStack,     DA_DRWA|DA_32

; 显存首地址-文本模式（数据段）
LABEL_DESC_VIDEO:   Descriptor 0xb8000, 0xffff,         DA_DRW|DA_DPL3

; 0~4G
LABEL_DESC_FLAT_C:  Descriptor 0,        0fffffh, DA_CR|DA_32|DA_LIMIT_4K
; 0~4G
LABEL_DESC_FLAT_RW: Descriptor 0,        0fffffh, DA_DRW|DA_LIMIT_4K

GdtLen	equ	$ - LABEL_GDT	; GDT长度
GdtPtr	dw	GdtLen - 1		; GDT界限
		dd	0				; GDT基地址(会由初始化代码设置)

; GDT选择子
SelectorNormal		equ LABEL_DESC_NORMAL   - LABEL_GDT
SelectorCode32		equ LABEL_DESC_CODE32   - LABEL_GDT
SelectorCode16		equ LABEL_DESC_CODE16   - LABEL_GDT
SelectorData		equ LABEL_DESC_DATA	    - LABEL_GDT
SelectorStack		equ LABEL_DESC_STACK    - LABEL_GDT
SelectorVideo       equ LABEL_DESC_VIDEO    - LABEL_GDT

SelectorFlatC		equ	LABEL_DESC_FLAT_C	- LABEL_GDT
SelectorFlatRW		equ	LABEL_DESC_FLAT_RW	- LABEL_GDT



; Data数据段
[SECTION .data]
ALIGN	32
[BITS	32]
LABEL_DATA:
; 实模式下使用这些符号
; 字符串
_szPMMessage:			db	"In Protect Mode now. ^-^", 0Ah, 0Ah, 0	; 进入保护模式后显示此字符串
_szMemChkTitle:			db	"BaseAddrL BaseAddrH LengthLow LengthHigh   Type", 0Ah, 0	; 进入保护模式后显示此字符串
_szRAMSize			db	"RAM size:", 0
_szReturn			db	0Ah, 0
; 变量
_wSPValueInRealMode		dw	0
_dwMCRNumber:			dd	0	; Memory Check Result
_dwDispPos:			dd	(80 * 6 + 0) * 2	; 屏幕第 6 行, 第 0 列。
_dwMemSize:			dd	0
_ARDStruct:			; Address Range Descriptor Structure
	_dwBaseAddrLow:		dd	0
	_dwBaseAddrHigh:	dd	0
	_dwLengthLow:		dd	0
	_dwLengthHigh:		dd	0
	_dwType:		dd	0
_PageTableNumber		dd	0

_MemChkBuf:	times	256	db	0

; 保护模式下使用这些符号
szPMMessage		equ	_szPMMessage	- $$
szMemChkTitle		equ	_szMemChkTitle	- $$
szRAMSize		equ	_szRAMSize	- $$
szReturn		equ	_szReturn	- $$
dwDispPos		equ	_dwDispPos	- $$
dwMemSize		equ	_dwMemSize	- $$
dwMCRNumber		equ	_dwMCRNumber	- $$
ARDStruct		equ	_ARDStruct	- $$
	dwBaseAddrLow	equ	_dwBaseAddrLow	- $$
	dwBaseAddrHigh	equ	_dwBaseAddrHigh	- $$
	dwLengthLow	equ	_dwLengthLow	- $$
	dwLengthHigh	equ	_dwLengthHigh	- $$
	dwType		equ	_dwType		- $$
MemChkBuf		equ	_MemChkBuf	- $$
PageTableNumber		equ	_PageTableNumber- $$

DataLen			equ	$ - LABEL_DATA



; 全局堆栈段
ALIGN	32
[BITS	32]
LABEL_STACK:
times 512 db 0
TopOfStack      equ $-LABEL_STACK-1
