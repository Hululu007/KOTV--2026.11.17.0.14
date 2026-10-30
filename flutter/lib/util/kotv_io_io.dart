export 'dart:io';

import 'dart:ffi';
import 'dart:io' show Platform;

import 'package:ffi/ffi.dart';

/// 本机 CPU 架构：x64 / arm64 / arm / x86 / riscv64，与引擎 `runtime.Arch` 同一套写法。
String kotvHostArch() {
  final abi = Abi.current().toString();
  final arch = abi.substring(abi.indexOf('_') + 1);
  return arch == 'ia32' ? 'x86' : arch;
}

typedef _SetEnvNative = Int32 Function(Pointer<Utf16>, Pointer<Utf16>);
typedef _SetEnvDart = int Function(Pointer<Utf16>, Pointer<Utf16>);

/// Windows：写本进程环境变量，给同进程的原生插件读（Platform.environment 只读）。其它平台不做。
void kotvSetProcessEnv(String name, String value) {
  if (!Platform.isWindows) return;
  final setEnv = DynamicLibrary.open('kernel32.dll')
      .lookupFunction<_SetEnvNative, _SetEnvDart>('SetEnvironmentVariableW');
  final n = name.toNativeUtf16(allocator: calloc);
  final v = value.toNativeUtf16(allocator: calloc);
  try {
    setEnv(n, v);
  } finally {
    calloc.free(n);
    calloc.free(v);
  }
}
