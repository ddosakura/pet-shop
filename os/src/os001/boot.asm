org		0x7c00           ; 引导程序装载地址

; FAT12格式软盘专用 2880扇区*512字节 = 1440*1024字节 1_44标准
; C18-H2-S18
; 结合《30天自制操作系统》与
; https://baike.baidu.com/item/FAT12/4958604
; 的说明
; IPL - 启动程序加载器
jmp		entry            ; BS_jmpBoot <3B>
nop
DB      "Sakura  "       ; BS_OEMName 厂商名(启动区名) <8B>
DW      0x0200           ; 扇区(sector)大小[标准-512字节]
DB      0x01             ; 簇(cluster)大小[标准-1个扇区]
DW      0x0001           ; Boot记录占用扇区数(FAT起始位置)[一般第一个]
DB      0x02             ; FAT表个数[必须2]
DW      0x00e0           ; 根目录文件数最大值(根目录大小)[一般224项]
DW      0x0b40           ; BPB_TotSec16 扇区总数(磁盘大小)[标准-2880扇区]
DB      0xf0             ; 介质描述符(磁盘种类)[必须]
DW      0x0009           ; 每FAT扇区数(FAT长度)[必须9扇区]
DW      0x0012           ; 每个磁道(track)的扇区数[必须18扇区]
DW      0x0002           ; 磁头数[必须2]
DD      0x00000000       ; 隐藏扇区数(不使用分区)[必须0]
DD      0x00000b40       ; 如果BPB_TotSec16是0，这个值记录扇区数(重写一次)
DB      0x00             ; 中断13的驱动器号
DB      0x00             ; 未使用
DB      0x29             ; 扩展引导标记
DD      0x00000000       ; 卷序列号
DB      "SakuraTmpOS"    ; 卷标(磁盘名称) <11B>
DB      "FAT12   "       ; 文件系统类型(磁盘格式名称) <8B>

; 引导代码、数据及其他填充字符等
entry:
mov     ax, cs
mov     ds, ax
;mov     ss, ax
;mov     sp, 0x7c00
call    hw

loading:                    ; 装载扇区
;mov     dl, 0              ; DL=驱动器; 软盘-00H~7FH; 硬盘-80H~0FFH
;mov     ch, 0              ; 柱面0
;mov     dh, 0              ; 磁头0
;mov     cl, 2              ; 扇区2
;mov     al, 1              ; 扇区数1
;mov     ax, 0x0820
;mov     es, ax
;mov     bx, 0              ; es:bx 缓冲地址
;mov     ah, 0x02           ; BIOS中断-读扇区
mov     ax, payload
mov     es, ax
mov     bx, 0
mov     cx, 0x0002
mov     dx, 0
readloop:
mov     si, 0               ; 计数器
retry:
mov     ax, 0x0201
int     0x13                ; BIOS中断;成功CF=0|AH=00H|AL=传输扇区数;失败AH=状态代码
;jc      error
jnc     next
call    error
inc     si
cmp     si, retry_times
jae     error_final
mov     ah, 0
mov     dl, 0
int     0x13                ; BIOS中断
jmp     retry               ; 防止读取错误，重试
next:                       ; 因为0-0只剩下17个扇区，所以不一个个扇区读入有点麻烦
mov     ax, es
add     ax, 0x20            ; 512*1
mov     es, ax
inc     cl
cmp     cl, 18
jbe     readloop
mov     cl, 1
inc     dh
cmp     dh, 2
jb      readloop
call    part_success
mov     dh, 0
inc     ch
cmp     ch, CNUM
jb      readloop

ok_finnal:
mov     ax, cs
mov     es, ax
mov     ax, okmsg
mov     bp, ax              ; es:bp 串地址
mov     cx, lenstr4         ; 串长度
call    print
jmp     os_seg:os_entry

error_final:
mov     ax, cs
mov     es, ax
mov     ax, errmsg2
mov     bp, ax              ; es:bp 串地址
mov     cx, lenstr3         ; 串长度
call    print

;jmp     $
fin:
hlt                         ; CPU暂停
jmp     fin

print:
push    ax
push    bx
push    dx
;mov     bh, 0              ; 页号(页码)
;mov     al, 0x01           ; 输出方式：只含显示字符;显示属性在BL中;显示后，光标位置改变 
;mov     bl, 0x0c           ; 属性：黑底红字
;mov     ah, 0x13           ; BIOS中断-屏幕输出
;mov     dx, 0              ; (DH、DL)＝坐标(行、列) 
mov     ax, 0x1301
mov     bx, 0x000c
mov     dx, [pos]
int     0x10                ; BIOS中断
inc     dh
mov     [pos], dx
pop     dx
pop     bx
pop     ax
ret

hw:
mov     ax, cs
mov     es, ax
mov     ax, msg
mov     bp, ax              ; es:bp 串地址
mov     cx, lenstr          ; 串长度
call    print
ret

error:
push    ax
push    cx
push    es
push    bp
mov     ax, cs
mov     es, ax
mov     ax, errmsg
mov     bp, ax              ; es:bp 串地址
mov     cx, lenstr2         ; 串长度
call    print
pop     bp
pop     es
pop     cx
pop     ax
ret

part_success:
push    ax
push    bx
push    cx
push    es
push    bp
mov     ax, [scount]
inc     ax
mov     [scount], ax
mov     bl, pre_load
div     bl
cmp     ah, 0
je      pps2
pps1:
pop     bp
pop     es
pop     cx
pop     bx
pop     ax
ret
pps2:
mov     ax, cs
mov     es, ax
mov     ax, smsg
mov     bp, ax              ; es:bp 串地址
mov     cx, lensmsg         ; 串长度
call    print
jmp     pps1

; var
pos     dw 0
scount  dw 0

; const
msg     db  "Hello World! loading..."
lenstr  equ $-msg
errmsg  db  "Load Error! retry..."
lenstr2 equ $-errmsg
errmsg2 db  "Load Error! Payload may be damaged!"
lenstr3 equ $-errmsg2
okmsg   db  "Load Success!"
lenstr4 equ $-okmsg
smsg    db  "Load 5 sector success!"
lensmsg equ $-smsg

; 内存分布图
; 0x0000 0000 ~ 0x0000 03ff  ->  Real Mode IVT (1kb)
; 0x0000 0000 ~ 0x0000 03ff  ->  BDA - BIOS data area (256b)
;
; 0x0000 0500 ~ 0x0000 7bff  ->  可用区 (~30kb)
; 0x0000 7c00 ~ 0x0000 7dff  ->  引导区 (512b)
; 0x0000 7e00 ~ 0x0007 ffff  ->  可用区 (480.5kb)
;
; 0x0008 0000 ~ 0x0009 fbff  ->  可用区 (~120kb 视EBDA大小)
; 0x0009 fc00 ~ 0x0009 ffff  ->  EBDA (1kb) 在0xA0000前 (<128Kb) [通常1~8kb]
;
; 0x000a 0000 ~ 0x000f ffff  ->  video mem ROM (384kb)

; 一个柱面 +0x4800
; 一个磁道 +0x2400
; 一个扇区 +0x0200

; setting
;payload     equ 0x7e0         ; 载入地址(ES段，真实地址*16)
; 载入到 0xfe00 时莫名内存全置0了，跳过这个
os_seg      equ 0xfe0         ; 起始段地址
payload     equ os_seg+0x20
CNUM        equ 10            ; 读入的柱面数 180kb (+0x2d000)
os_entry    equ 0x4400        ; os.bin 的入口地址为 os_seg*16+os_entry

retry_times equ 5             ; 最大重试次数
pre_load    equ 5             ; 每多少个扇区显示一次载入信息

times   510-($-$$) db 0
dw      0xaa55                ; 结束标志
