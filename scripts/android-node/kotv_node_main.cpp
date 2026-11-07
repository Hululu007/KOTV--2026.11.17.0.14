// 安卓 Node 启动器：可执行 PIE，链到 libnode.so，调用 node::Start。
// 打成 libkotv_node.so 放进 jniLibs；首次用到猫爪源时再按 ABI 下载 libnode.so。
#include "node.h"

int main(int argc, char* argv[]) {
  return node::Start(argc, argv);
}
