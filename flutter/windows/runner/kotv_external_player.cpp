#include "kotv_external_player.h"

#include "utils.h"

#include <shellapi.h>
#include <shobjidl.h>
#include <windows.h>

#include <algorithm>
#include <cwchar>
#include <set>
#include <string>
#include <vector>

namespace {

std::wstring Utf16FromUtf8(const std::string& utf8) {
  if (utf8.empty()) return {};
  const int n = ::MultiByteToWideChar(CP_UTF8, 0, utf8.data(),
                                      static_cast<int>(utf8.size()), nullptr, 0);
  if (n <= 0) return {};
  std::wstring out(static_cast<size_t>(n), L'\0');
  ::MultiByteToWideChar(CP_UTF8, 0, utf8.data(), static_cast<int>(utf8.size()),
                        out.data(), n);
  return out;
}

std::wstring ToLower(std::wstring s) {
  for (wchar_t& c : s) {
    if (c >= L'A' && c <= L'Z') c = static_cast<wchar_t>(c - L'A' + L'a');
  }
  return s;
}

bool EndsWithInsensitive(const std::wstring& path, const wchar_t* suffix) {
  const size_t n = wcslen(suffix);
  if (path.size() < n) return false;
  return _wcsicmp(path.c_str() + (path.size() - n), suffix) == 0;
}

bool IsBrowserExecutable(const std::wstring& path) {
  const std::wstring lower = ToLower(path);
  static const wchar_t* kBrowsers[] = {
      L"chrome.exe",   L"msedge.exe", L"firefox.exe", L"iexplore.exe",
      L"opera.exe",    L"brave.exe",  L"vivaldi.exe", L"safari.exe",
      L"chromium.exe",
  };
  for (const wchar_t* name : kBrowsers) {
    if (EndsWithInsensitive(lower, name)) return true;
  }
  // Chrome 等常以 Application\chrome.exe 出现；再按路径片段排除。
  if (lower.find(L"\\google\\chrome\\") != std::wstring::npos) return true;
  if (lower.find(L"\\microsoft\\edge\\") != std::wstring::npos) return true;
  if (lower.find(L"\\mozilla firefox\\") != std::wstring::npos) return true;
  return false;
}

bool IsSelfExecutable(const std::wstring& path) {
  wchar_t self[MAX_PATH] = {};
  const DWORD n = ::GetModuleFileNameW(nullptr, self, MAX_PATH);
  if (n == 0 || n >= MAX_PATH) return false;
  return _wcsicmp(path.c_str(), self) == 0;
}

void CollectHandlersForExt(const wchar_t* ext, std::set<std::wstring>* seen,
                           flutter::EncodableList* out) {
  IEnumAssocHandlers* enumerator = nullptr;
  if (FAILED(::SHAssocEnumHandlers(ext, ASSOC_FILTER_NONE, &enumerator)) ||
      enumerator == nullptr) {
    return;
  }
  IAssocHandler* handler = nullptr;
  while (enumerator->Next(1, &handler, nullptr) == S_OK && handler != nullptr) {
    LPWSTR name_path = nullptr;
    LPWSTR ui_name = nullptr;
    if (SUCCEEDED(handler->GetName(&name_path)) && name_path != nullptr) {
      std::wstring exe(name_path);
      ::CoTaskMemFree(name_path);
      if (EndsWithInsensitive(exe, L".exe") && !IsBrowserExecutable(exe) &&
          !IsSelfExecutable(exe) && seen->insert(ToLower(exe)).second) {
        std::wstring label;
        if (SUCCEEDED(handler->GetUIName(&ui_name)) && ui_name != nullptr) {
          label = ui_name;
          ::CoTaskMemFree(ui_name);
          ui_name = nullptr;
        }
        if (label.empty()) {
          const size_t slash = exe.find_last_of(L"\\/");
          label = slash == std::wstring::npos ? exe : exe.substr(slash + 1);
        }
        flutter::EncodableMap row;
        row[flutter::EncodableValue("id")] =
            flutter::EncodableValue(Utf8FromUtf16(exe.c_str()));
        row[flutter::EncodableValue("name")] =
            flutter::EncodableValue(Utf8FromUtf16(label.c_str()));
        out->push_back(flutter::EncodableValue(row));
      }
    }
    if (ui_name != nullptr) ::CoTaskMemFree(ui_name);
    handler->Release();
    handler = nullptr;
  }
  enumerator->Release();
}

}  // namespace

flutter::EncodableList KotvListExternalVideoApps() {
  flutter::EncodableList out;
  std::set<std::wstring> seen;
  static const wchar_t* kExts[] = {L".mp4",  L".mkv",  L".avi", L".mov",
                                   L".wmv",  L".ts",   L".m3u8", L".m3u"};
  for (const wchar_t* ext : kExts) {
    CollectHandlersForExt(ext, &seen, &out);
  }
  std::sort(out.begin(), out.end(),
            [](const flutter::EncodableValue& a, const flutter::EncodableValue& b) {
              const auto* ma = std::get_if<flutter::EncodableMap>(&a);
              const auto* mb = std::get_if<flutter::EncodableMap>(&b);
              if (ma == nullptr || mb == nullptr) return false;
              const auto ia = ma->find(flutter::EncodableValue("name"));
              const auto ib = mb->find(flutter::EncodableValue("name"));
              const std::string* sa =
                  ia == ma->end() ? nullptr : std::get_if<std::string>(&ia->second);
              const std::string* sb =
                  ib == mb->end() ? nullptr : std::get_if<std::string>(&ib->second);
              return (sa == nullptr ? "" : *sa) < (sb == nullptr ? "" : *sb);
            });
  return out;
}

bool KotvOpenExternalVideoApp(const std::string& exe_utf8,
                              const std::string& url_utf8) {
  const std::wstring exe = Utf16FromUtf8(exe_utf8);
  const std::wstring url = Utf16FromUtf8(url_utf8);
  if (exe.empty() || url.empty()) return false;
  // 参数加引号，避免地址里的 &/? 被命令行拆开。
  const std::wstring params = L"\"" + url + L"\"";
  const HINSTANCE hi = ::ShellExecuteW(nullptr, L"open", exe.c_str(),
                                       params.c_str(), nullptr, SW_SHOWNORMAL);
  return reinterpret_cast<INT_PTR>(hi) > 32;
}
