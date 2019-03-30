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
; 非一致代码段 (32位)
LABEL_DESC_CODE_DEST: Descriptor 0,		SegCodeDestLen-1,DA_C+DA_32

; Data数据段
LABEL_DESC_DATA:    Descriptor 0,       DataLen-1,      DA_DRW
; Stack 全局堆栈段 (32位)
LABEL_DESC_STACK:   Descriptor 0,       TopOfStack,     DA_DRWA+DA_32

; Test数据段
LABEL_DESC_TEST:    Descriptor 0x500000,0xffff,         DA_DRW

; 显存首地址-文本模式（数据段）
LABEL_DESC_VIDEO:   Descriptor 0xb8000, 0xffff,         DA_DRW
; 显存首地址-彩色（数据段）
; https://blog.csdn.net/pizi0475/article/details/8739577
LABEL_DESC_VIDEOX:  Descriptor 0xa0000,	0xffff,         DA_DRW

; LDT段 (局部描述符)
LABEL_DESC_LDT:		Descriptor 0,		LDTLen-1,		DA_LDT

; 门
; 							目标选择子,			 偏移,DCount,属性
LABEL_CALL_GATE_TEST: Gate	SelectorCodeDest, 	0,	0,	DA_386CGate+DA_DPL0

GdtLen	equ	$ - LABEL_GDT	; GDT长度
GdtPtr	dw	GdtLen - 1		; GDT界限
		dd	0				; GDT基地址(会由初始化代码设置)

; GDT选择子
SelectorNormal		equ LABEL_DESC_NORMAL   - LABEL_GDT
SelectorCode32		equ LABEL_DESC_CODE32   - LABEL_GDT
SelectorCode16		equ LABEL_DESC_CODE16   - LABEL_GDT
SelectorCodeDest	equ	LABEL_DESC_CODE_DEST- LABEL_GDT
SelectorData		equ LABEL_DESC_DATA	    - LABEL_GDT
SelectorStack		equ LABEL_DESC_STACK    - LABEL_GDT
SelectorTest		equ LABEL_DESC_TEST	    - LABEL_GDT
SelectorVideo       equ LABEL_DESC_VIDEO    - LABEL_GDT
SelectorVideoX		equ LABEL_DESC_VIDEOX   - LABEL_GDT
; LDT段 的 GDT选择子
SelectorLDT			equ LABEL_DESC_LDT		- LABEL_GDT
; 门选择子
SelectorCallGateTest equ LABEL_CALL_GATE_TEST-LABEL_GDT



; Data数据段
[SECTION .data]
ALIGN	32
[BITS	32]
LABEL_DATA:
SPValueInRealMode	dw	0
; 字符串
PMMessage:      db	"In Protect Mode now. ^-^", 0	; 在保护模式中显示
OffsetPMMessage equ	PMMessage-$$
StrTest:        db	"ABCDEFGHIJKLMNOPQRSTUVWXYZ", 0
OffsetStrTest   equ	StrTest-$$
DataLen         equ	$-LABEL_DATA



; 全局堆栈段
ALIGN	32
[BITS	32]
LABEL_STACK:
times 512 db 0
TopOfStack      equ $-LABEL_STACK-1
