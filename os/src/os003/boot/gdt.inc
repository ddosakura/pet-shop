; GDT
;                              段基址,   段界限,          属性
; 空描述符
LABEL_GDT:          Descriptor 0,		0,              0
; Normal 描述符
LABEL_DESC_NORMAL:  Descriptor 0,       0xffff,         DA_DRW
; 非一致代码段 (32位)
LABEL_DESC_CODE32:  Descriptor 0,       SegCode32Len-1, DA_C+DA_32
; 非一致代码段 (16位)
LABEL_DESC_CODE16:  Descriptor 0,       0ffffh,         DA_C

; Data数据段
LABEL_DESC_DATA:    Descriptor 0,       DataLen-1,      DA_DRW
; Stack 全局堆栈段 (32位)
LABEL_DESC_STACK:   Descriptor 0,       TopOfStack,     DA_DRWA+DA_32

; 显存首地址-文本模式（数据段）
LABEL_DESC_VIDEO:   Descriptor 0xb8000, 0xffff,         DA_DRW+DA_DPL3

;Page Directory
LABEL_DESC_PAGE_DIR:Descriptor PageDirBase, 4095,		DA_DRW
;Page Tables
LABEL_DESC_PAGE_TBL:Descriptor PageTblBase, 1023,		DA_DRW|DA_LIMIT_4K

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
SelectorPageDir		equ	LABEL_DESC_PAGE_DIR	- LABEL_GDT
SelectorPageTbl		equ	LABEL_DESC_PAGE_TBL	- LABEL_GDT



; Data数据段
[SECTION .data]
ALIGN	32
[BITS	32]
LABEL_DATA:
SPValueInRealMode	dw	0
; 字符串
PMMessage:      db	"In Protect Mode now. ^-^", 0	; 在保护模式中显示
OffsetPMMessage equ	PMMessage-$$
DataLen         equ	$-LABEL_DATA



; 全局堆栈段
ALIGN	32
[BITS	32]
LABEL_STACK:
times 512 db 0
TopOfStack      equ $-LABEL_STACK-1
