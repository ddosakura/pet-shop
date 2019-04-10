// #include <iostream>
// using namespace std;

// see: https://www.cnblogs.com/Braveliu/p/4231818.html

#include<cstdio>

int main() {
    // int a = 20, b = 10;
    // auto fun = [=, &b](int c)->int { return b += a+c;};
    // cout << "fun(100) :" << fun(100) << endl;
    
    int a = 10;
    auto fun = [=](int b)->int {return a+b;};
    int c = fun(100);
    printf("%d", c);
}
