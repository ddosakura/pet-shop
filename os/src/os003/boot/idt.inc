; IDT

ALIGN	32
[BITS	32]
LABEL_IDT:
; 门                        目标选择子,            偏移, DCount, 属性
%rep 32
		Gate	SelectorCode32, SpuriousHandler,      0, DA_386IGate
%endrep
.020h:		Gate	SelectorCode32,    ClockHandler,      0, DA_386IGate
%rep 95
		Gate	SelectorCode32, SpuriousHandler,      0, DA_386IGate
%endrep
.080h:		Gate	SelectorCode32,  UserIntHandler,      0, DA_386IGate

IdtLen      equ $-LABEL_IDT
IdtPtr      dw  IdtLen-1        ; 段界限
            dd  0               ; 基地址
