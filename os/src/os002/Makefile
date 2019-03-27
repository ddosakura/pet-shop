os.img:
	bximage
#mkfs:
#	mkfs.vfat os.img

# xxd
# u 字母大写
# a 自动隐藏0
# g 每多少字节一起显示
# c 每行多少字节
# s 偏移字节
# l 字节数限制
see:
	xxd -a -l 512 os.img
see1:
	xxd -a -s +0x2600 -l 512 os.img
see2:
	xxd -a -s +0x4200 -l 512 os.img
see3:
	xxd -a -s +0x4400 -l 512 os.img

bochs:
	sudo bochs -f boot.bxrc
bochsq:
	sudo bochs -q -f boot.bxrc

clean:
	rm *.log
