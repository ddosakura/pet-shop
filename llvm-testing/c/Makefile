g:
	clang -emit-llvm -S main.c -o main.ll
as:
	llvm-as main.ll -o main.bc
run:
	lli main.bc
dis:
	llvm-dis main.bc -o main.ll
llc:
	llc main.bc -o  main.s
build:
	# as main.s -o main.out; chmod +x main.out
	as main.s -o main.o; ld -o main.out main.o; chmod +x main.out