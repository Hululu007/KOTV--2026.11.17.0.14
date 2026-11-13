// Header-only: pick a DXGI adapter that can HEVC-decode when one exists.
// Used by FVP (D3D11 device) and media_kit ANGLE (D3D11 texture device).
// Prefer capability, not vendor name. Skip Microsoft Basic Render Driver.
// Probe once per process — D3D11CreateDevice + video-profile enum is heavy on Win7.
#pragma once

#include <d3d11.h>
#include <dxgi.h>
#include <wrl/client.h>

#include <cstring>

namespace kotv_dxgi {
namespace detail {

// Local HEVC profile GUIDs (older SDKs / Win7 toolchains may lack the macros).
static const GUID kHevcMain = {
    0x5b11d51b,
    0x2f4c,
    0x4452,
    {0xbc, 0xc3, 0x09, 0xfe, 0xd4, 0xb6, 0x05, 0xf9}};
static const GUID kHevcMain10 = {
    0x107af0e0,
    0xef1a,
    0x4d19,
    {0xab, 0xa8, 0x67, 0xa1, 0x63, 0x07, 0x3d, 0x13}};

inline bool GuidEq(const GUID& a, const GUID& b) {
  return std::memcmp(&a, &b, sizeof(GUID)) == 0;
}

inline bool AdapterSupportsHevc(IDXGIAdapter* adapter) {
  if (!adapter) return false;
  Microsoft::WRL::ComPtr<ID3D11Device> device;
  Microsoft::WRL::ComPtr<ID3D11DeviceContext> ctx;
  static const D3D_FEATURE_LEVEL kLevels[] = {
      D3D_FEATURE_LEVEL_11_0,
      D3D_FEATURE_LEVEL_10_1,
      D3D_FEATURE_LEVEL_10_0,
      D3D_FEATURE_LEVEL_9_3,
  };
  if (FAILED(D3D11CreateDevice(adapter, D3D_DRIVER_TYPE_UNKNOWN, nullptr, 0,
                               kLevels, ARRAYSIZE(kLevels), D3D11_SDK_VERSION,
                               &device, nullptr, &ctx)) ||
      !device) {
    return false;
  }
  Microsoft::WRL::ComPtr<ID3D11VideoDevice> video;
  if (FAILED(device.As(&video)) || !video) return false;
  const UINT n = video->GetVideoDecoderProfileCount();
  for (UINT i = 0; i < n; ++i) {
    GUID profile = {};
    if (FAILED(video->GetVideoDecoderProfile(i, &profile))) continue;
    if (GuidEq(profile, kHevcMain) || GuidEq(profile, kHevcMain10)) return true;
  }
  return false;
}

inline bool IsBasicRender(const DXGI_ADAPTER_DESC& desc) {
  return desc.VendorId == 0x1414 && desc.DeviceId == 0x8c;
}

}  // namespace detail

// Returns AddRef'd adapter (caller Release), or nullptr if none.
// First call probes; later calls reuse the same adapter (including nullptr).
inline IDXGIAdapter* PickAdapterPreferHevc() {
  static Microsoft::WRL::ComPtr<IDXGIAdapter> cached;
  static bool probed = false;
  if (probed) {
    IDXGIAdapter* out = cached.Get();
    if (out) out->AddRef();
    return out;
  }
  probed = true;

  Microsoft::WRL::ComPtr<IDXGIFactory> factory;
  if (FAILED(CreateDXGIFactory(__uuidof(IDXGIFactory), &factory)) || !factory) {
    return nullptr;
  }
  Microsoft::WRL::ComPtr<IDXGIAdapter> first_hw;
  Microsoft::WRL::ComPtr<IDXGIAdapter> hevc;
  for (UINT i = 0;; ++i) {
    Microsoft::WRL::ComPtr<IDXGIAdapter> adapter;
    if (factory->EnumAdapters(i, &adapter) == DXGI_ERROR_NOT_FOUND) break;
    if (!adapter) continue;
    DXGI_ADAPTER_DESC desc = {};
    if (FAILED(adapter->GetDesc(&desc))) continue;
    if (detail::IsBasicRender(desc)) continue;
    if (!first_hw) first_hw = adapter;
    if (!hevc && detail::AdapterSupportsHevc(adapter.Get())) hevc = adapter;
  }
  cached = hevc ? hevc : first_hw;
  IDXGIAdapter* out = cached.Get();
  if (out) out->AddRef();
  return out;
}

}  // namespace kotv_dxgi
