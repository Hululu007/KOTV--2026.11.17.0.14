// 安卓 Node 启动器：可执行 PIE，链到同目录 libnode.so，调用 node::Start。
// 打成 libkotv_node.so，与 libnode.so 一并放入 jniLibs。
#include "node.h"

int main(int argc, char* argv[]) {
  return node::Start(argc, argv);
}
