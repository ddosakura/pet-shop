%include 'config.inc'

jmp		entry
nop
%include 'fat12_1_44.inc'

; move to 0x90000
entry:
mov     ax, boot_seg
mov     ds, ax
mov     ax, init_seg
mov     es, ax
mov     cx, 0x100           ; 256个字 -> 512B
sub     si, si              ; 置0
sub     di, di              ; 置0
cld                         ; 方向标志置1
rep movsw                   ; loop until cx=0; 移动 ds:si -> es:di
jmp     init_seg:go

go:
mov     ax, cs
mov     ds, ax
;mov     es, ax

mov     ax, load_seg
mov     es, ax
mov     bx, 0
mov     cx, 0x0002
mov     dx, 0
readloop:
mov     ax, 0x0201
int     0x13
jnc     next
error_final:
hlt
jmp     error_final

next:
mov     ax, es
add     ax, 0x20
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
cmp     ch, bootload
jb      readloop

ok_finnal:
jmp     head_seg:0

times   510-($-$$) db 0
dw      0xaa55
