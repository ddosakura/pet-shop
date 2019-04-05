# 保护模式

+ A20 地址线
+ 描述符表 (Descriptor Table)
    + GDT (Global DT) 全局描述符表
    + LDT (Local DT) 局部描述符表
    + IDT (Interrupt DT) 中断描述符表
        + 中断门描述符 (特殊的调用门)
        + 陷阱门描述符 (特殊的调用门)
        + 任务门描述符 - linux中没用到
+ 特权级 (Privilege Level)
    + 级别
        + 0 操作系统内核  
        + 1、2 服务  
        + 3 应用程序  
    + 分类
        + CPL (Current PL) 当前特权级
        + DPL (Descriptor PL) 描述符特权级
        + RPL (Requested PL) 请求特权级
+ 门描述符 (Gate)
    + 调用门 (Call Gates) - 可实现特权级由低到高转移
    + 中断门 (Interrupt Gates)
    + 陷阱门 (Trap Gates)
    + 任务门 (Task Gates)
+ [中断/异常](https://wiki.osdev.org/Exceptions)
    + 类型
        + Fault - 故障(可更正异常) - 返回地址是产生fault的指令
        + Trap - 陷阱 - 返回地址是产生trap的指令之后的指令
        + Abort - 中止(严重错误)
        + Interrupt - 中断
            + 外部中断 (硬件)
            + int n （类似调用门）
