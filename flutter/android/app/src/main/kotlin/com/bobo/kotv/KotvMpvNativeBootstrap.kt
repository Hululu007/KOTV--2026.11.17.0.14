package com.bobo.kotv

import android.content.Context
import android.content.pm.PackageManager
import android.content.res.AssetManager
import android.os.Build
import android.util.Log
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.io.InputStream

/**
 * Media3 [androidx.media3.mpvplayer.MpvPlayer] 启动前的本机辅助：
 * API25 等机型在 System.loadLibrary("mpv") 前 FORCE_LOAD 应用内 Vulkan 1.1 符号桩。
 */
object KotvMpvNativeBootstrap {
  private const val TAG = "KotvMpvNative"
  private const val ASSET_ROOT = "mpv-libs"
  /** FEATURE_VULKAN_HARDWARE_VERSION：Vulkan 1.2 特性位。 */
  private const val VULKAN_1_2 = 0x00402000

  @Volatile private var deviceVulkanCapable: Boolean? = null
  @Volatile private var vulkanStubLoaded = false

  external fun nativeLoadGlobal(absolutePath: String): Boolean

  /** 设备报告 Vulkan ≥1.2。 */
  @JvmStatic
  @Synchronized
  fun isDeviceVulkanCapable(context: Context?): Boolean {
    deviceVulkanCapable?.let { return it }
    return try {
      val c = resolveContext(context)
      val pm = c?.packageManager
      if (pm == null) {
        Log.w(TAG, "PackageManager unavailable; defer Vulkan capability probe")
        false
      } else {
        val ok = pm.hasSystemFeature(PackageManager.FEATURE_VULKAN_HARDWARE_VERSION, VULKAN_1_2)
        deviceVulkanCapable = ok
        ok
      }
    } catch (t: Throwable) {
      Log.w(TAG, "Unable to detect device Vulkan support", t)
      false
    }
  }

  /**
   * 在首次加载 Media3 libmpv 之前 FORCE_LOAD 应用内 Vulkan stub。
   * 与设置里是否开启 Vulkan 渲染无关；桩只满足链接。
   */
  @JvmStatic
  @Synchronized
  fun preloadVulkanStubEarly(context: Context?): Boolean {
    if (vulkanStubLoaded) return true
    return try {
      if (isDeviceVulkanCapable(context)) return false
      val app = resolveContext(context) ?: return false
      val abi = chooseAbi(app.assets) ?: return false
      val root = app.getDir("mpv-libs", Context.MODE_PRIVATE)
      val dir = File(root, abi)
      if (!dir.exists() && !dir.mkdirs()) {
        Log.w(TAG, "Unable to create $dir")
        return false
      }
      copyLibrary(app.assets, abi, "vulkan", dir)
      try {
        System.loadLibrary("kotv_dl")
      } catch (e: UnsatisfiedLinkError) {
        Log.w(TAG, "loadLibrary(kotv_dl) failed during stub preload", e)
        return false
      }
      val extracted = File(dir, System.mapLibraryName("vulkan"))
      loadVulkanStubGlobal(extracted)
      vulkanStubLoaded
    } catch (t: Throwable) {
      Log.w(TAG, "preloadVulkanStubEarly failed", t)
      false
    }
  }

  /**
   * 真机有系统 Vulkan 时删掉 APK 抽出的 stub（旧包 jniLibs 残留），
   * 否则 libmpv DT_NEEDED 仍优先绑到空壳。
   */
  @JvmStatic
  @Synchronized
  fun removeAppVulkanStubIfPresent(context: Context?) {
    try {
      val app = context?.applicationContext ?: return
      val stub = File(app.applicationInfo.nativeLibraryDir, "libvulkan.so")
      if (!stub.isFile) return
      val len = stub.length()
      if (len > 0L && len < 2_000_000L && stub.delete()) {
        Log.i(TAG, "removed app libvulkan stub from ${stub.absolutePath} ($len bytes)")
      } else {
        Log.w(TAG, "could not remove app libvulkan stub ${stub.absolutePath} len=$len")
      }
    } catch (t: Throwable) {
      Log.w(TAG, "removeAppVulkanStubIfPresent failed", t)
    }
  }

  /** stub 已占位时不能开真正的 Vulkan 渲染。 */
  @JvmStatic
  fun isVulkanStubLoaded(): Boolean = vulkanStubLoaded

  private fun loadVulkanStubGlobal(vulkanSo: File) {
    if (!vulkanSo.isFile) {
      Log.w(TAG, "libvulkan stub missing; libmpv may fail on API25")
      return
    }
    if (!nativeLoadGlobal(vulkanSo.absolutePath)) {
      Log.w(TAG, "nativeLoadGlobal(libvulkan) failed; trying System.load $vulkanSo")
      System.load(vulkanSo.absolutePath)
    } else {
      Log.i(TAG, "libvulkan stub loaded globally from ${vulkanSo.absolutePath}")
    }
    vulkanStubLoaded = true
  }

  private fun resolveContext(context: Context?): Context? {
    if (context == null) return null
    return try {
      context.applicationContext ?: context
    } catch (_: Throwable) {
      context
    }
  }

  private fun chooseAbi(assets: AssetManager): String? {
    for (abi in Build.SUPPORTED_ABIS) {
      if (assetExists(assets, abi, "vulkan")) return abi
    }
    return if (assetExists(assets, "armeabi-v7a", "vulkan")) "armeabi-v7a" else null
  }

  private fun assetExists(assets: AssetManager, abi: String, lib: String): Boolean =
    try {
      assets.open(assetPath(abi, lib), AssetManager.ACCESS_STREAMING).use { true }
    } catch (_: IOException) {
      false
    }

  private fun assetPath(abi: String, lib: String): String =
    "$ASSET_ROOT/$abi/${System.mapLibraryName(lib)}"

  private fun copyLibrary(assets: AssetManager, abi: String, lib: String, dir: File) {
    val name = System.mapLibraryName(lib)
    val dest = File(dir, name)
    assets.open(assetPath(abi, lib)).use { input ->
      writeStream(input, dest)
    }
  }

  private fun writeStream(input: InputStream, dest: File) {
    FileOutputStream(dest).use { out ->
      val buf = ByteArray(64 * 1024)
      while (true) {
        val n = input.read(buf)
        if (n <= 0) break
        out.write(buf, 0, n)
      }
      out.fd.sync()
    }
  }
}
