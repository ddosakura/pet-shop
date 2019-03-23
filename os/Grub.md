# Grub

步骤：
1. 格式化u盘
```bash
sudo cfdisk /dev/sdc
sudo mkfs.ext4 -L data /dev/sdc1
sudo mkfs.ext4 -L boot /dev/sdc2
```
2. 安装Grub
```bash
mkdir ./boot
sudo mount /dev/sdc2 ./boot
sudo grub-install --root-directory=path/to/boot --no-floppy /dev/sdc
sudo umount ./boot
```
3. 配置Grub
    + grub2采用`grub.cfg`而不是`menu.lst`(用于grub/grub4dos)
```lst
# menu.lst
title 1 haribote
map --mem /haribote.img (fd0)
map --hook
chainloader (fd0)+1
rootnoverify (fd0)
map --floppies=1
boot
```
```cfg
# grub.cfg
# memdisk 工具来自 syslinux，注意与 grub2 的 memdisk 区分
menuentry 'haribote' {
    linux16 /boot/tools/memdisk
    initrd16 /boot/res/haribote.img
}
```
4. 测试将u盘打成iso放入虚拟机(找不到小u盘，弄出来的iso太大了，跳过)
```bash
sudo dd bs=4M if=/dev/sdc of=./grub-test.iso status=progress
```

参考：
+ https://www.cnblogs.com/wunaozai/p/3854875.html
+ https://blog.csdn.net/deng_sai/article/details/50066831
+ https://blog.csdn.net/mao0514/article/details/51218522
+ https://wiki.archlinux.org/index.php/GRUB
+ https://wiki.archlinux.org/index.php/GRUB_Legacy
+ http://blog.chinaunix.net/uid-20801390-id-1839224.html
+ https://blog.csdn.net/victorwjw/article/details/76680054
+ http://bbs.wuyou.net/forum.php?mod=viewthread&tid=373411
+ https://www.librehat.com/grub2-boot-windows-pe-and-otheriso-file/

其他：
+ https://forum.ubuntu.org.cn/viewtopic.php?t=347622
+ https://bbs.deepin.org/forum.php?mod=viewthread&tid=141725
+ https://github.com/a1ive/grub2-filemanager
+ http://bbs.wuyou.net/forum.php?mod=viewthread&tid=377735
