boot_seg    equ 0x07c0
init_seg    equ 0x9000

core_seg    equ 0x1000
load_seg    equ core_seg+0x0020
head_seg    equ core_seg+0x0440

bootload    equ 10              ; 读入的柱面数 180kb (+0x2d000)

; BOOT_INFO
CYLS	EQU		0x0ff0			; 设定启动区
LEDS	EQU		0x0ff1
VMODE	EQU		0x0ff2			; 颜色位数
SCRNX	EQU		0x0ff4			; screenX
SCRNY	EQU		0x0ff6			; screenY
VRAM	EQU		0x0ff8			; 图像缓冲区的开始地址

BOTPAK	EQU		0x00280000
DSKCAC	EQU		0x00100000
DSKCAC0	EQU		0x00008000
