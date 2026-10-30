/// Web 最小 dart:io stub（仅覆盖本项目用到的 Platform / 少量 File API）。
class Platform {
  static bool get isAndroid => false;
  static bool get isIOS => false;
  static bool get isMacOS => false;
  static bool get isWindows => false;
  static bool get isLinux => false;
  static String get pathSeparator => '/';
  static String get operatingSystem => 'web';
  static String get operatingSystemVersion => '';
  static String get resolvedExecutable => '';
  static String get script => '';
  static String get executable => '';
  static Map<String, String> get environment => const {};
}

class File {
  File(this.path);
  final String path;
  Directory get parent {
    final i = path.lastIndexOf('/');
    if (i <= 0) return Directory('/');
    return Directory(path.substring(0, i));
  }

  bool existsSync() => false;
  Future<bool> exists() async => false;
  Future<String> readAsString({encoding}) async => '';
  Future<File> writeAsString(String contents, {encoding, mode, flush}) async => this;
  void writeAsStringSync(String contents, {encoding, mode, flush}) {}
  Future<File> delete({bool recursive = false}) async => this;
  void deleteSync({bool recursive = false}) {}
  Future<File> rename(String newPath) async => File(newPath);
  Future<int> length() async => 0;
  int lengthSync() => 0;
}

class Directory {
  Directory(this.path);
  final String path;
  static Directory get current => Directory('.');
  static Directory get systemTemp => Directory('/tmp');
  Directory get parent => Directory(path);
  bool existsSync() => false;
  Future<bool> exists() async => false;
  Future<Directory> create({bool recursive = false}) async => this;
  void createSync({bool recursive = false}) {}
  Stream<FileSystemEntity> list({bool recursive = false, bool followLinks = true}) =>
      const Stream.empty();
  List<FileSystemEntity> listSync({bool recursive = false, bool followLinks = true}) =>
      const [];
}

class FileSystemEntity {
  String get path => '';
  static bool isDirectorySync(String path) => false;
  Future<FileSystemEntity> delete({bool recursive = false}) async => this;
  void deleteSync({bool recursive = false}) {}
}

class Process {
  static Future<ProcessResult> run(String executable, List<String> arguments,
          {String? workingDirectory, Map<String, String>? environment, bool includeParentEnvironment = true, bool runInShell = false, encoding, stderrEncoding}) async =>
      ProcessResult(0, 1, '', 'unsupported on web');
  static Future<Process> start(String executable, List<String> arguments,
          {String? workingDirectory, Map<String, String>? environment, bool includeParentEnvironment = true, bool runInShell = false, ProcessStartMode mode = ProcessStartMode.normal}) async =>
      throw UnsupportedError('Process.start unsupported on web');
}

class ProcessResult {
  ProcessResult(this.pid, this.exitCode, this.stdout, this.stderr);
  final int pid;
  final int exitCode;
  final dynamic stdout;
  final dynamic stderr;
}

enum ProcessStartMode { normal, detached, detachedWithStdio }

/// dart:io FileMode，供 writeAsStringSync 命名参数在 Web 编译期解析。
enum FileMode { read, write, append, writeOnly, writeOnlyAppend }

class ProcessSignal {
  static const ProcessSignal sigterm = ProcessSignal._();
  static const ProcessSignal sigkill = ProcessSignal._();
  const ProcessSignal._();
}

class HttpClient {
  Future<HttpClientRequest> getUrl(Uri url) async => throw UnsupportedError('use package:http on web');
  Future<HttpClientRequest> openUrl(String method, Uri url) async => throw UnsupportedError('use package:http on web');
  void close({bool force = false}) {}
}

abstract class HttpClientRequest {
  Future<HttpClientResponse> close();
}

abstract class HttpClientResponse {
  int get statusCode;
}

Never exit(int code) => throw UnsupportedError('exit unsupported on web');

/// 浏览器里拿不到可靠的 CPU 架构。
String kotvHostArch() => '';

void kotvSetProcessEnv(String name, String value) {}
