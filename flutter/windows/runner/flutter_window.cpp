#include "flutter_window.h"

#include <optional>
#include <string>

#include <flutter/standard_method_codec.h>
#include <shellapi.h>
#include <windows.h>

#include "flutter/generated_plugin_registrant.h"
#include "kotv_iface_rx.h"

namespace {

// Windows：写成临时 m3u，再用「打开方式」交给本机播放器（VLC / PotPlayer / MPC / mpv 等）。
// 不可直接 ShellExecute http(s)：默认常落到浏览器。macOS/Linux 则把地址直接交给选定应用。
bool KotvOpenExternalVideoChooser(const std::string& url, const std::string& title) {
  wchar_t temp_dir[MAX_PATH];
  const DWORD dir_len = GetTempPathW(MAX_PATH, temp_dir);
  if (dir_len == 0 || dir_len >= MAX_PATH) return false;

  wchar_t temp_file[MAX_PATH];
  if (GetTempFileNameW(temp_dir, L"kotv", 0, temp_file) == 0) return false;

  // GetTempFileName 生成 .tmp；改成 .m3u 便于关联到播放器。
  std::wstring m3u_path(temp_file);
  const size_t dot = m3u_path.find_last_of(L'.');
  if (dot != std::wstring::npos) {
    m3u_path.resize(dot);
  }
  m3u_path += L".m3u";
  MoveFileExW(temp_file, m3u_path.c_str(), MOVEFILE_REPLACE_EXISTING);

  std::string body = "#EXTM3U\r\n";
  if (!title.empty()) {
    body += "#EXTINF:-1,";
    body += title;
    body += "\r\n";
  } else {
    body += "#EXTINF:-1,KO\r\n";
  }
  body += url;
  body += "\r\n";

  HANDLE file = CreateFileW(m3u_path.c_str(), GENERIC_WRITE, 0, nullptr,
                            CREATE_ALWAYS, FILE_ATTRIBUTE_TEMPORARY, nullptr);
  if (file == INVALID_HANDLE_VALUE) return false;
  DWORD written = 0;
  const BOOL wrote = WriteFile(file, body.data(),
                               static_cast<DWORD>(body.size()), &written, nullptr);
  CloseHandle(file);
  if (!wrote || written != body.size()) return false;

  // 系统「打开方式」对话框，让用户选本机视频应用。
  const HINSTANCE hi = ShellExecuteW(nullptr, L"openas", m3u_path.c_str(),
                                     nullptr, nullptr, SW_SHOWNORMAL);
  return reinterpret_cast<INT_PTR>(hi) > 32;
}

}  // namespace

FlutterWindow::FlutterWindow(const flutter::DartProject& project)
    : project_(project) {}

FlutterWindow::~FlutterWindow() {}

bool FlutterWindow::OnCreate() {
  if (!Win32Window::OnCreate()) {
    return false;
  }

  RECT frame = GetClientArea();

  // The size here must match the window dimensions to avoid unnecessary surface
  // creation / destruction in the startup path.
  flutter_controller_ = std::make_unique<flutter::FlutterViewController>(
      frame.right - frame.left, frame.bottom - frame.top, project_);
  // Ensure that basic setup of the controller was successful.
  if (!flutter_controller_->engine() || !flutter_controller_->view()) {
    return false;
  }
  RegisterPlugins(flutter_controller_->engine());
  RegisterHostChannel();
  SetChildContent(flutter_controller_->view()->GetNativeWindow());

  flutter_controller_->engine()->SetNextFrameCallback([&]() {
    this->Show();
  });

  // Flutter can complete the first frame before the "show window" callback is
  // registered. The following call ensures a frame is pending to ensure the
  // window is shown. It is a no-op if the first frame hasn't completed yet.
  flutter_controller_->ForceRedraw();

  return true;
}

void FlutterWindow::RegisterHostChannel() {
  host_channel_ = std::make_unique<flutter::MethodChannel<flutter::EncodableValue>>(
      flutter_controller_->engine()->messenger(), "kotv_host",
      &flutter::StandardMethodCodec::GetInstance());
  host_channel_->SetMethodCallHandler(
      [](const flutter::MethodCall<flutter::EncodableValue>& call,
         std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result) {
        if (call.method_name() == "getInterfaceRxBytes") {
          result->Success(flutter::EncodableValue(KotvInterfaceRxBytes()));
        } else if (call.method_name() == "getMemoryInfo") {
          MEMORYSTATUSEX st;
          st.dwLength = sizeof(st);
          flutter::EncodableMap mem;
          if (GlobalMemoryStatusEx(&st)) {
            mem[flutter::EncodableValue("totalBytes")] =
                flutter::EncodableValue(static_cast<int64_t>(st.ullTotalPhys));
            mem[flutter::EncodableValue("availBytes")] =
                flutter::EncodableValue(static_cast<int64_t>(st.ullAvailPhys));
          } else {
            mem[flutter::EncodableValue("totalBytes")] = flutter::EncodableValue(0);
            mem[flutter::EncodableValue("availBytes")] = flutter::EncodableValue(0);
          }
          result->Success(flutter::EncodableValue(mem));
        } else if (call.method_name() == "hasExternalVideoApp") {
          result->Success(flutter::EncodableValue(true));
        } else if (call.method_name() == "openExternalVideoChooser") {
          const auto* args = std::get_if<flutter::EncodableMap>(call.arguments());
          std::string url;
          std::string title;
          if (args != nullptr) {
            const auto url_it = args->find(flutter::EncodableValue("url"));
            if (url_it != args->end()) {
              if (const auto* s = std::get_if<std::string>(&url_it->second)) {
                url = *s;
              }
            }
            const auto title_it = args->find(flutter::EncodableValue("title"));
            if (title_it != args->end()) {
              if (const auto* s = std::get_if<std::string>(&title_it->second)) {
                title = *s;
              }
            }
          }
          if (url.empty()) {
            result->Error("url", "empty url");
            return;
          }
          result->Success(flutter::EncodableValue(KotvOpenExternalVideoChooser(url, title)));
        } else {
          result->NotImplemented();
        }
      });
}

void FlutterWindow::OnDestroy() {
  host_channel_ = nullptr;
  if (flutter_controller_) {
    flutter_controller_ = nullptr;
  }

  Win32Window::OnDestroy();
}

LRESULT
FlutterWindow::MessageHandler(HWND hwnd, UINT const message,
                              WPARAM const wparam,
                              LPARAM const lparam) noexcept {
  // Give Flutter, including plugins, an opportunity to handle window messages.
  if (flutter_controller_) {
    std::optional<LRESULT> result =
        flutter_controller_->HandleTopLevelWindowProc(hwnd, message, wparam,
                                                      lparam);
    if (result) {
      return *result;
    }
  }

  switch (message) {
    case WM_FONTCHANGE:
      flutter_controller_->engine()->ReloadSystemFonts();
      break;
  }

  return Win32Window::MessageHandler(hwnd, message, wparam, lparam);
}
