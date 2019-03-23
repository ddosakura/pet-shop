# BIOS 中断

## INT 0x10 显示服务(Video Service)

AH=<功能>

### AH=0x0e 《30天自制操作系统》

> 功能描述：在Teletype模式下显示字符
>
> 入口参数：AH＝0EH 
> > AL＝字符  
> > BH＝页码  
> > BL＝前景色(图形模式)  
>
> 出口参数：无 

```
AL=char code
BH=0
BL=color code
```

### AH=0x13 《自己动手写操作系统》

> 功能描述：在Teletype模式下显示字符串 
>
> 入口参数：AH＝13H 
> > BH＝页码  
> > BL＝属性(若AL=00H或01H)  
> > CX＝显示字符串长度  
> > (DH、DL)＝坐标(行、列)  
> > ES:BP＝显示字符串的地址  
> > AL＝显示输出方式   
> > + 0——字符串中只含显示字符，其显示属性在BL中。显示后，光标位置不变 
> > + 1——字符串中只含显示字符，其显示属性在BL中。显示后，光标位置改变 
> > + 2——字符串中含显示字符和显示属性。显示后，光标位置不变 
> > + 3——字符串中含显示字符和显示属性。显示后，光标位置改变 
>
> 出口参数：无 