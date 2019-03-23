# OS

## Books

+ 《30天自制操作系统》川合秀实
    + [x] day1
+ 《自己动手写操作系统》于渊
    + [x] chapter2
+ 《操作系统设计与实现》
    + [ ] ...

## Warning

### bochs

1. `'keyboard_mapping' is deprecated - use 'keyboard' option instead.`

> See: https://blog.csdn.net/MT1232/article/details/80178361
>
> 配置文件中keyboard_mapping改为：keyboard:  keymap=/usr/share/bochs/keymaps/x11-pc-us.map

2. `couldn't open ROM image file '/usr/share/vgabios/vgabios.bin'`

> See: https://stackoverflow.com/questions/25667779/bochs-vgaromimage-error
>
> `ls /usr/share/bochs`

3. 黑屏

> See: https://bbs.csdn.net/topics/340062048
>
> 是调试模式，键入 `c` `<enter>`
