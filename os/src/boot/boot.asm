org		0x7c00

mov     ax, cs
mov     ds, ax
;mov     ss, ax
;mov     sp, 0x7c00
call    hw

loading:                    ; 装载扇区
;mov     dl, 0              ; DL＝驱动器，00H~7FH：软盘；80H~0FFH：硬盘
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
cmp     si, 5
jae     error_final
mov     ah, 0
mov     dl, 0
int     0x13                ; BIOS中断
jmp     retry
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
jmp     fin

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
pop     cs
pop     ax
ret

pos     dw 0
msg     db  "Hello World! loading..."
lenstr  equ $-msg
errmsg  db  "Load Error! retry..."
lenstr2 equ $-errmsg
errmsg2 db  "Load Error! Payload may be damaged!"
lenstr3 equ $-errmsg2
okmsg   db  "Load Success!"
lenstr4 equ $-okmsg
payload equ 0x8200
CNUM    equ 10              ; 读入的柱面数

TIMES   510-($-$$) DB 0
DW      0xaa55
