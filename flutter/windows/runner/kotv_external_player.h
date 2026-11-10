#ifndef RUNNER_KOTV_EXTERNAL_PLAYER_H_
#define RUNNER_KOTV_EXTERNAL_PLAYER_H_

#include <flutter/encodable_value.h>

#include <string>

// 枚举本机可处理常见视频扩展名的应用（排除浏览器），供 Flutter 网格选择。
flutter::EncodableList KotvListExternalVideoApps();

// 用选定可执行文件打开播放地址（原始 http(s)/代理 URL，不做 m3u 包装）。
bool KotvOpenExternalVideoApp(const std::string& exe_utf8, const std::string& url_utf8);

#endif  // RUNNER_KOTV_EXTERNAL_PLAYER_H_
