export 'dart:io';

import 'dart:ffi' show Abi;

/// 本机 CPU 架构：x64 / arm64 / arm / x86 / riscv64，与引擎 `runtime.Arch` 同一套写法。
String kotvHostArch() {
  final abi = Abi.current().toString();
  final arch = abi.substring(abi.indexOf('_') + 1);
  return arch == 'ia32' ? 'x86' : arch;
}
