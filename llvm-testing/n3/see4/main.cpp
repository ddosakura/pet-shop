#include<cstdio>

struct Ctx {
    long long a1;
    long long a2;
};

long long bar(Ctx *a, long long b1, long long b2) {
    printf("%lld %lld %lld %lld\n", a->a1, a->a2, b1, b2);
    return (a->a1+b1)*(a->a2+b2);
}

auto foo(long long a1, long long a2) {
    Ctx c;
    c.a1 = a1;
    c.a2 = a2;
    // warning: address of stack memory associated with local variable 'c' returned [-Wreturn-stack-address]
    return &c;
}

int main() {
    auto ctx = foo(4, 3);
    printf("%lld\n", bar(ctx, 6, 7));
    return 0;
}
