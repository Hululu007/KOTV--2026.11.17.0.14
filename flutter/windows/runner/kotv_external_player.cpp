#include "kotv_external_player.h"

#include "utils.h"

#include <windows.h>
#include <shellapi.h>
#include <shobjidl.h>
#include <wincodec.h>

#include <algorithm>
#include <cstring>
#include <cwchar>
#include <set>
#include <string>
#include <vector>

namespace {

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

bool IsVideoHandlerExe(const std::wstring& exe) {
  return EndsWithInsensitive(exe, L".exe") && !IsBrowserExecutable(exe) &&
         !IsSelfExecutable(exe);
}

// HICON 画到 side×side 的 32bpp BGRA，再经 WIC 编成 PNG（与 macOS/Linux 列表契约一致）。
std::vector<uint8_t> HiconToPng(HICON icon, int side) {
  if (icon == nullptr || side <= 0) return {};

  BITMAPINFO bmi = {};
  bmi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
  bmi.bmiHeader.biWidth = side;
  bmi.bmiHeader.biHeight = -side;  // top-down
  bmi.bmiHeader.biPlanes = 1;
  bmi.bmiHeader.biBitCount = 32;
  bmi.bmiHeader.biCompression = BI_RGB;

  void* bits = nullptr;
  HDC screen = ::GetDC(nullptr);
  if (screen == nullptr) return {};
  HDC mem = ::CreateCompatibleDC(screen);
  HBITMAP dib =
      ::CreateDIBSection(mem, &bmi, DIB_RGB_COLORS, &bits, nullptr, 0);
  if (mem == nullptr || dib == nullptr || bits == nullptr) {
    if (dib != nullptr) ::DeleteObject(dib);
    if (mem != nullptr) ::DeleteDC(mem);
    ::ReleaseDC(nullptr, screen);
    return {};
  }

  HGDIOBJ old = ::SelectObject(mem, dib);
  const size_t nbytes = static_cast<size_t>(side) * static_cast<size_t>(side) * 4;
  std::memset(bits, 0, nbytes);
  ::DrawIconEx(mem, 0, 0, icon, side, side, 0, nullptr, DI_NORMAL);
  ::SelectObject(mem, old);
  ::DeleteDC(mem);
  ::ReleaseDC(nullptr, screen);

  std::vector<uint8_t> png;
  IWICImagingFactory* factory = nullptr;
  HRESULT hr = ::CoCreateInstance(CLSID_WICImagingFactory, nullptr,
                                  CLSCTX_INPROC_SERVER, IID_PPV_ARGS(&factory));
  if (FAILED(hr) || factory == nullptr) {
    ::DeleteObject(dib);
    return {};
  }

  IWICBitmap* bitmap = nullptr;
  const UINT stride = static_cast<UINT>(side * 4);
  hr = factory->CreateBitmapFromMemory(
      static_cast<UINT>(side), static_cast<UINT>(side),
      GUID_WICPixelFormat32bppBGRA, stride, static_cast<UINT>(nbytes),
      static_cast<BYTE*>(bits), &bitmap);
  ::DeleteObject(dib);
  if (FAILED(hr) || bitmap == nullptr) {
    if (factory != nullptr) factory->Release();
    return {};
  }

  IStream* stream = nullptr;
  hr = ::CreateStreamOnHGlobal(nullptr, TRUE, &stream);
  if (FAILED(hr) || stream == nullptr) {
    bitmap->Release();
    factory->Release();
    return {};
  }

  IWICBitmapEncoder* encoder = nullptr;
  hr = factory->CreateEncoder(GUID_ContainerFormatPng, nullptr, &encoder);
  if (SUCCEEDED(hr) && encoder != nullptr) {
    hr = encoder->Initialize(stream, WICBitmapEncoderNoCache);
    IWICBitmapFrameEncode* frame = nullptr;
    if (SUCCEEDED(hr)) {
      hr = encoder->CreateNewFrame(&frame, nullptr);
    }
    if (SUCCEEDED(hr) && frame != nullptr) {
      hr = frame->Initialize(nullptr);
      if (SUCCEEDED(hr)) {
        hr = frame->SetSize(static_cast<UINT>(side), static_cast<UINT>(side));
      }
      WICPixelFormatGUID format = GUID_WICPixelFormat32bppBGRA;
      if (SUCCEEDED(hr)) {
        hr = frame->SetPixelFormat(&format);
      }
      if (SUCCEEDED(hr)) {
        hr = frame->WriteSource(bitmap, nullptr);
      }
      if (SUCCEEDED(hr)) {
        hr = frame->Commit();
      }
      if (SUCCEEDED(hr)) {
        hr = encoder->Commit();
      }
      frame->Release();
    }
    encoder->Release();
  }

  if (SUCCEEDED(hr)) {
    STATSTG stat = {};
    if (SUCCEEDED(stream->Stat(&stat, STATFLAG_NONAME)) &&
        stat.cbSize.QuadPart > 0 && stat.cbSize.QuadPart < 4 * 1024 * 1024) {
      const ULONG size = static_cast<ULONG>(stat.cbSize.QuadPart);
      LARGE_INTEGER zero = {};
      stream->Seek(zero, STREAM_SEEK_SET, nullptr);
      png.resize(size);
      ULONG read = 0;
      if (FAILED(stream->Read(png.data(), size, &read)) || read != size) {
        png.clear();
      }
    }
  }

  stream->Release();
  bitmap->Release();
  factory->Release();
  return png;
}

std::vector<uint8_t> IconPngFromPath(const std::wstring& path, int index,
                                     int side) {
  if (path.empty() || side <= 0) return {};
  // 勿用标识符 small/large：windows.h / rpcndr.h 可能把 small 宏成 char，MSVC 会在 `!=` 处炸。
  HICON large_icon = nullptr;
  HICON small_icon = nullptr;
  // SHDefExtractIcon：Win7+；按目标边长抽大图标。
  const HRESULT hr = ::SHDefExtractIconW(path.c_str(), index, 0, &large_icon,
                                         &small_icon, static_cast<UINT>(side));
  HICON use = nullptr;
  if (SUCCEEDED(hr) || hr == S_FALSE) {
    use = large_icon != nullptr ? large_icon : small_icon;
  }
  if (use == nullptr) {
    // 回退：经典 ExtractIconEx（索引 0）。
    if (index == 0) {
      ::ExtractIconExW(path.c_str(), 0, &large_icon, &small_icon, 1);
      use = large_icon != nullptr ? large_icon : small_icon;
    }
  }
  std::vector<uint8_t> png;
  if (use != nullptr) {
    png = HiconToPng(use, side);
  }
  if (large_icon != nullptr) ::DestroyIcon(large_icon);
  if (small_icon != nullptr && small_icon != large_icon) ::DestroyIcon(small_icon);
  return png;
}

std::vector<uint8_t> AssocHandlerIconPng(IAssocHandler* handler,
                                         const std::wstring& exe, int side) {
  if (handler != nullptr) {
    wchar_t loc[MAX_PATH] = {};
    int index = 0;
    if (SUCCEEDED(handler->GetIconLocation(loc, MAX_PATH, &index)) &&
        loc[0] != L'\0') {
      std::vector<uint8_t> png = IconPngFromPath(loc, index, side);
      if (!png.empty()) return png;
    }
  }
  return IconPngFromPath(exe, 0, side);
}

// visit(exe, label, handler) 返回 false 时停止枚举。
template <typename Visit>
void ForEachVideoHandler(Visit&& visit) {
  std::set<std::wstring> seen;
  static const wchar_t* kExts[] = {L".mp4",  L".mkv",  L".avi", L".mov",
                                   L".wmv",  L".ts",   L".m3u8", L".m3u"};
  for (const wchar_t* ext : kExts) {
    IEnumAssocHandlers* enumerator = nullptr;
    if (FAILED(::SHAssocEnumHandlers(ext, ASSOC_FILTER_NONE, &enumerator)) ||
        enumerator == nullptr) {
      continue;
    }
    IAssocHandler* handler = nullptr;
    bool stop = false;
    while (!stop && enumerator->Next(1, &handler, nullptr) == S_OK &&
           handler != nullptr) {
      LPWSTR name_path = nullptr;
      if (SUCCEEDED(handler->GetName(&name_path)) && name_path != nullptr) {
        std::wstring exe(name_path);
        ::CoTaskMemFree(name_path);
        if (IsVideoHandlerExe(exe) && seen.insert(ToLower(exe)).second) {
          std::wstring label;
          LPWSTR ui_name = nullptr;
          if (SUCCEEDED(handler->GetUIName(&ui_name)) && ui_name != nullptr) {
            label = ui_name;
            ::CoTaskMemFree(ui_name);
          }
          if (label.empty()) {
            const size_t slash = exe.find_last_of(L"\\/");
            label = slash == std::wstring::npos ? exe : exe.substr(slash + 1);
          }
          if (!visit(exe, label, handler)) stop = true;
        }
      }
      handler->Release();
      handler = nullptr;
    }
    enumerator->Release();
    if (stop) return;
  }
}

}  // namespace

bool KotvHasExternalVideoApp() {
  bool found = false;
  ForEachVideoHandler(
      [&](const std::wstring&, const std::wstring&, IAssocHandler*) {
        found = true;
        return false;
      });
  return found;
}

flutter::EncodableList KotvListExternalVideoApps() {
  flutter::EncodableList out;
  ForEachVideoHandler([&](const std::wstring& exe, const std::wstring& label,
                          IAssocHandler* handler) {
    flutter::EncodableMap row;
    row[flutter::EncodableValue("id")] =
        flutter::EncodableValue(Utf8FromUtf16(exe.c_str()));
    row[flutter::EncodableValue("name")] =
        flutter::EncodableValue(Utf8FromUtf16(label.c_str()));
    std::vector<uint8_t> png = AssocHandlerIconPng(handler, exe, 64);
    if (!png.empty()) {
      row[flutter::EncodableValue("icon")] =
          flutter::EncodableValue(std::move(png));
    }
    out.push_back(flutter::EncodableValue(row));
    return true;
  });
  std::sort(out.begin(), out.end(),
            [](const flutter::EncodableValue& a,
               const flutter::EncodableValue& b) {
              const auto* ma = std::get_if<flutter::EncodableMap>(&a);
              const auto* mb = std::get_if<flutter::EncodableMap>(&b);
              if (ma == nullptr || mb == nullptr) return false;
              const auto ia = ma->find(flutter::EncodableValue("name"));
              const auto ib = mb->find(flutter::EncodableValue("name"));
              const std::string* sa =
                  ia == ma->end() ? nullptr
                                  : std::get_if<std::string>(&ia->second);
              const std::string* sb =
                  ib == mb->end() ? nullptr
                                  : std::get_if<std::string>(&ib->second);
              return (sa == nullptr ? "" : *sa) < (sb == nullptr ? "" : *sb);
            });
  return out;
}

bool KotvOpenExternalVideoApp(const std::string& exe_utf8,
                              const std::string& url_utf8) {
  const std::wstring exe = Utf16FromUtf8(exe_utf8);
  const std::wstring url = Utf16FromUtf8(url_utf8);
  if (exe.empty() || url.empty()) return false;
  const std::wstring params = L"\"" + url + L"\"";
  const HINSTANCE hi = ::ShellExecuteW(nullptr, L"open", exe.c_str(),
                                       params.c_str(), nullptr, SW_SHOWNORMAL);
  return reinterpret_cast<INT_PTR>(hi) > 32;
}
