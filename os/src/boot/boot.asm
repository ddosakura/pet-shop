org		0x7c00

mov     ax, cs
mov     ds, ax
mov     es, ax
call    print
;jmp     $
fin:
hlt                         ; CPU暂停
jmp     fin

print:
mov     ax, msg
mov     bp, ax              ; es:bp 串地址
mov     cx, lenstr          ; 串长度
mov     ax, 0x1301          ; AH=0x13 AL=0x01
mov     bx, 0x000c          ; 页号BH=0 黑底红字BL=0x0c
mov     dl, 0
int     0x10                ; BIOS中断 屏幕输出
ret

msg:
string  db  "Hello World!"
lenstr  equ $-string

TIMES   510-($-$$) DB 0
DW      0xaa55
