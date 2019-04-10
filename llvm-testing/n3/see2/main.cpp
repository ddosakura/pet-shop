#include<cstdio>

auto foo(int a) {
    auto bar = [=](int b)->int {return a+b;};
    return bar;
}
int main() {
    auto bar1 = foo(4);
    printf("%d\n", bar1(6));
    // printf("%d\n", bar(-4));
    auto bar2 = foo(3);
    printf("%d\n", bar2(7));
    return 0;
}

//int xBar(int b) {}
//auto xFoo() {
//    auto pf = &xBar;
//    return pf;
//}
//int main() {
//    xFoo()(1);
//}
