extern void _io_hlt();

void HariMain() {
fin:
	_io_hlt();
	goto fin;
}
