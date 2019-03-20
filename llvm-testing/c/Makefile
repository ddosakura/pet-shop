g:
	clang -emit-llvm -S main.c -o main.ll
as:
	llvm-as main.ll -o main.bc
dis:
	llvm-dis main.bc -o main.ll
llc:
	llc main.bc -o  main.s
build:
	as main.s -o main.out; chmod +x main.out