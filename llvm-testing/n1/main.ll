; ModuleID = 'main.bc'
source_filename = "main.src"

define i64 @ip() {
input:
  ret i64 2
}

define void @op(i64 %x) {
output:
  unreachable
}

define i64 @add(i64 %x, i64 %y) {
add:
  %0 = add i64 2, 2
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
