#ifndef RUNNER_KOTV_EXTERNAL_PLAYER_H_
#define RUNNER_KOTV_EXTERNAL_PLAYER_H_

#include <flutter/encodable_value.h>

#include <string>

// 是否已有至少一个可用的本机视频应用（找到即停，不造完整列表）。
bool KotvHasExternalVideoApp();

// 枚举本机可处理常见视频扩展名的应用（排除浏览器），带 64px PNG 图标（失败则无 icon 字段）。
flutter::EncodableList KotvListExternalVideoApps();

// 用选定可执行文件打开播放地址（当前播放地址原样传入）。
bool KotvOpenExternalVideoApp(const std::string& exe_utf8, const std::string& url_utf8);

#endif  // RUNNER_KOTV_EXTERNAL_PLAYER_H_
