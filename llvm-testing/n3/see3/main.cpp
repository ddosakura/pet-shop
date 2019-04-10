#include<cstdio>

auto foo(long long a1, long long a2) {
    auto bar = [=](long long b1, long long b2)->long long {
        return (a1+b1)*(a2+b2);
    };
    return bar;
}
int main() {
    auto bar = foo(4, 3);
    printf("%lld\n", bar(6, 7));
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
