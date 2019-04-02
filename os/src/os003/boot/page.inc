PageDirBase0		equ	200000h	; 页目录开始地址:	2M
PageTblBase0		equ	201000h	; 页表开始地址:		2M +  4K
PageDirBase1		equ	210000h	; 页目录开始地址:	2M + 64K
PageTblBase1		equ	211000h	; 页表开始地址:		2M + 64K + 4K

LinearAddrDemo	equ	00401000h
ProcFoo		equ	00401000h
ProcBar		equ	00501000h
ProcPagingDemo	equ	00301000h
