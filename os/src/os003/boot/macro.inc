%macro printC 3
mov		ax, SelectorVideo
mov		gs, ax					; 视频段选择子(目的)
mov		edi, (80 * %1 + %2) * 2	; 屏幕第 %1 行, 第 %2 列。
mov		ah, 0Ch					; 0000: 黑底    1100: 红字
mov		al, %3					; char
mov		[gs:edi], ax
%endmacro
