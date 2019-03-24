# 汇编

+ [x86汇编指令集大全](https://blog.csdn.net/bjbz_cxy/article/details/79467688)
+ [数据传送指令详解](https://blog.csdn.net/shanyongxu/article/details/47726015)

## 寄存器

### 8位(16位寄存器高低位)

+ AL 累加 低位
+ CL 计数 低位
+ DL 数据 低位
+ BL 基址 低位
+ AH 累加 高位
+ CH 计数 高位
+ DH 数据 高位
+ BH 基址 高位

### 16位

+ [80x86](https://www.cnblogs.com/zhaoyl/archive/2012/05/15/2501972.html)

#### 通用寄存器

##### 数据寄存器

+ AX 累加
+ CX 计数
+ DX 数据
+ BX 基址 [可存内存地址]

##### 指针寄存器

+ SP 栈指针
+ BP 基址指针 [可存内存地址]

##### 变址寄存器

+ SI 源变址 [可存内存地址]
+ DI 目的变址 [可存内存地址]

#### 段寄存器

+ ES 附加段
+ CS 代码段
+ SS 栈段
+ DS 数据段（默认段寄存器）
+ FS (80386起增加的两个辅助段寄存器,减轻ES寄存器的负担)附加段
+ GS (80386起增加的两个辅助段寄存器,减轻ES寄存器的负担)附加段

> [ES:BX] 真实地址：ES*16+BX
>
> e.g
> ES=0xffff BX=0xffff
>
> Addr -> 0xffff0+0xffff 

#### 控制寄存器

##### IP 指令指针寄存器

##### FLAG 标志寄存器(PSW)

+ CF（Carry  FLag） - 进位标志（第 0 位）：
+ PF（Parity  FLag） - 奇偶标志（第 2 位）：
+ AF（Auxiliary  Carry  FLag） - 辅助进位标志（第 4 位）：
+ ZF（Zero  FLag） – 零标志（第 6 位）：
+ SF（Sign  FLag） - 符号标志（第 7 位）：
+ TF（Trap  FLag） - 追踪标志（第 8 位）：
+ IF（Interrupt-Enable  FLag） - 中断允许标志（第 9 位）：
+ DF（Direction  FLag） - 方向标志（第 10 位）：          
+ OF（OverFlow  FLag） - 溢出标志（第 11 位）：

|值|15|14|13|12|11|10| 9| 8| 7| 6| 5| 4| 3| 2| 1| 0|
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
|  |  |  |  |  |OF|DF|IF|TF|SF|ZF|  |AF|  |PF|  |CF|
| 0|  |  |  |  |NV|UP|  |  |PL|NZ|  |  |  |PO|  |NC|
| 1|  |  |  |  |OV|DN|  |  |NG|ZR|  |  |  |PE|  |CY|

### 32位(低16位与16位寄存器共用)

+ [#1](https://www.cnblogs.com/xiaojianliu/articles/8733512.html)
+ [80386](https://blog.csdn.net/u014774781/article/details/47707385)

#### 通用寄存器

+ EAX 累加
+ ECX 计数
+ EDX 数据
+ EBX 基址
+ ESP 栈指针
+ EBP 基址指针
+ ESI 源变址
+ EDI 目的变址

> 有些指令限制只能用其中某些寄存器做某种用途，例如除法指令idivl规定被除数在eax寄存器中，edx寄存器必须是0,而除数可以是任何寄存器中。计算结果的商数保存在eax寄存器中（覆盖被除数），余数保存在edx寄存器。

#### 段寄存器(CS、SS、DS、ES、FS、GS)
#### 指令指针寄存器和标志寄存器(EIP、EFLAGS)

|31|30|29|28|27|26|25|24|23|22|21| 20| 19|18|17|16|15|14|13|12|11|10| 9| 8| 7| 6| 5| 4| 3| 2| 1| 0|
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
|  |  |  |  |  |  |  |  |  |  |ID|VIP|VIF|AC|VM|RF|  |NT|IOPL(1)|IOPL(2)|OF|DF|IF|TF|SF|ZF|  |AF|  |PF|  |CF|

+ IOPL（I/O Privilege Level）是从80286开始出现的，占2个bit表示I/O特权级，如果当前特权级小于或等于IOPL，则可以执行I/O操作，否则将出现一个保护性异常。IOPL只能由特权级为0的程序或任务来修改。
+ NT（Nested Task）也是从80286开始出现的，表示嵌套任务，用于控制中断返回指令IRET，当NT=0时，用堆栈中保存的值恢复EFLAGS、CS和EIP，从而实现返回；若NT=1，则通过任务切换实现中断返回。
+ 下面的标志位是80386以后的CPU才有的标志。
    + VM（Virtual-8086 mode）表示虚拟8086模式，如果VM被置位且80386已出于保护模式下，则CPU切换到虚拟8086模式，此时，对段的任何操作又回到了实模式，如同在8086下运行一样。
    + RF（Resume flag）表示恢复标志(也叫重启标志)，与调试寄存器一起用于断点和单步操作，当RF＝1 时，下一条指令的任何调试故障将被忽略，不产生异常中断。当RF=0时，调试故障被接受，并产生异常中断。用于调试失败后，强迫程序恢复执行，在成功执行每条指令后，RF自动复位。
    + AC（Alignment check）表示对齐检查。这个标志是80486以后的CPU才有的。当AC=1且CR0中的AM=1时，允许存储器进行地址对齐检查，若发现地址未对齐，将产生异常中断。所谓地址对齐，是指当访问一个字（2字节长）时，其地址必须是偶数（2的倍数），当访问双字（4字节长）时，其地址必须是4的倍数。但是只有运行在特权级3的程序才执行地址对齐检查，特权级0、1、2忽略该标志。
+ 以下的三个标志是Pentium以后的CPU才有的。
    + VIF（Virtual interrupt flag）表示虚拟中断标志。当VIF=1时，可以使用虚拟中断，当VIF=0时不能使用虚拟中断。该标志要和下面的VIP和CR4中的VME配合使用。
    + VIP（Virtual interrupt pending flag）表示虚拟中断挂起标志。当VIP=1时，VIF有效，VIP=0时VIF无效。
    + ID（Identification flag）表示鉴别标志。该标志用来只是Pentium CPU是否支持CPUID的指令。

#### 系统表寄存器(GDTR、IDTR、LDTR、TR)
#### 控制寄存器(CR0、CR1、CR2、CR3、CR4)
#### 调试寄存器(DR0、DR1、DR2、DR3、DR4、DR5、DR6、DR7)
#### 测试寄存器(TR6、TR7)

### 64位

+ [x86-64](https://www.cnblogs.com/chenxuming/p/9689747.html)

+ 新增8个64位通用寄存器（整数寄存器）
    + R8、R9、R10、R11、R12、R13、R14和R15。
    + 可作为8位（R8B~R15B）、16位（R8W~R15W）或 32位寄存器（R8D~R15D）使用
+ 所有GPRs都从32位扩充到64位
    + 8个32位通用寄存器EAX、EBX、ECX、EDX、EBP、ESP、ESI和 EDI对应扩展寄存器分别为RAX、RBX、RCX、RDX、RBP、RSP、RSI和RDI
    + EBP、ESP、ESI和 EDI的低8位寄存器分别是BPL、SPL、SIL和DIL
    + 可兼容使用原AH、BH、CH和DH寄存器（使原来IA-32中的每个通用寄存器都可以是8位、16位、32位和64位，如：SIL、SI、ESI、RSI）

+ 指令可直接访问16个64位寄存器：RAX、RBX、RCX、RDX、RBP、RSP、RSI、RDI，以及R8~R15
+ 指令可直接访问16个32位寄存器：EAX、EBX、ECX、EDX、EBP、ESP、ESI、EDI，以及R8D~R15D
+ 指令可直接访问16个16位寄存器：AX、BX、CX、DX、BP、SP、SI、DI，以及R8W~R15W
+ 指令可直接访问16个8位寄存器：AL、BL、CL、DL、BPL、SPL、SIL、DIL，以及R8B~R15B
+ 为向后兼容，指令也可直接访问AH、BH、CH、DH
+ 通过寄存器传送参数，因而很多过程不用访问栈，因此，与IA-32不同，x86-64不需要帧指针寄存器，即RBP可用作普通寄存器使用
+ 程序计数器为64位寄存器RIP
