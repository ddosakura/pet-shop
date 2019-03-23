# OS

## Books & Docs

+ 《30天自制操作系统》川合秀实
    + [x] day1
+ 《自己动手写操作系统》于渊
    + [x] chapter2
+ 《操作系统设计与实现》
    + [ ] ...

### Bochs

+ [debug](https://www.jianshu.com/p/3b0588acbe16)

### Grub

[将Grub安装到u盘](./Grub.md)

### BIOS

+ [BIOS中断大全](https://blog.csdn.net/weixin_37656939/article/details/79684611)
    + 显示服务(Video Service——INT 10H)
    + 直接磁盘服务(Direct Disk Service——INT 13H)
    + 串行口服务(Serial Port Service——INT 14H)
    + 杂项系统服务(Miscellaneous System Service——INT 15H)
    + 键盘服务(Keyboard Service——INT 16H)
    + 并行口服务(Parallel Port Service——INT 17H)
    + 时钟服务(Clock Service——INT 1AH)
    + 直接系统服务(Direct System Service)
        + INT 00H —“0”作除数 
        + INT 01H —单步中断 
        + INT 02H —非屏蔽中断(NMI) 
        + INT 03H —断点中断 
        + INT 04H —算术溢出错误 
        + INT 05H —打印屏幕和BOUND越界 
        + INT 06H —非法指令错误 
        + INT 07H —处理器扩展无效 
        + INT 08H —时钟中断 
        + INT 09H —键盘输入 
        + INT 0BH —通信口(COM2:) 
        + INT 0CH —通信口(COM1:) 
        + INT 0EH —磁盘驱动器输入/输出 
        + INT 11H —读取设备配置 
        + INT 12H —读取常规内存大小(返回值AX为内存容量，以K为单位) 
        + INT 18H —ROM BASIC 
        + INT 19H —重启动系统 
        + INT 1BH —CTRL+BREAK处理程序 
        + INT 1CH —用户时钟服务 
        + INT 1DH —指向显示器参数表指针 
        + INT 1EH —指向磁盘驱动器参数表指针 
        + INT 1FH —指向图形字符模式表指针

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
