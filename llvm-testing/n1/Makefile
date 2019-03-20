g:
	go run main.go
as:
	llvm-as main.ll -o main.bc
dis:
	llvm-dis main.bc -o main.ll
llc:
	llc main.bc -o  main.s
build:
	as main.s -o main.out; chmod +x main.out

# See: https://blog.csdn.net/softee/article/details/41128667

hex:
	hexdump main.out
	# od -h main.out
