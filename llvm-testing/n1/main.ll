source_filename = "main.src"

@.str = private unnamed_addr constant [6 x i8] c"%lld\0A\00", align 1

declare i32 @printf(i8*, ...)

define i64 @ip() {
input:
	ret i64 2
}

define void @op(i64 %x) {
output:
	%0 = call i32 (i8*, ...) @printf(i8* getelementptr inbounds ([6 x i8], [6 x i8]* @.str, i32 0, i32 0), i64 %x)
	unreachable
}

define i64 @add(i64 %x, i64 %y) {
add:
	%0 = add i64 %x, %y
	ret i64 %0
}

define i64 @main() {
main:
	%0 = call i64 @ip()
	%1 = call i64 @ip()
	%2 = call i64 @add(i64 %0, i64 %1)
	call void @op(i64 %2)
	ret i64 0
}
