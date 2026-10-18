import Cocoa
import Darwin
import FlutterMacOS

@main
class AppDelegate: FlutterAppDelegate {
  override func applicationWillFinishLaunching(_ notification: Notification) {
    kotvApplyBundleEnvironment()
    super.applicationWillFinishLaunching(notification)
  }

  /// 运行时目录和 Vulkan ICD 用包内路径。主程序必须是本二进制，不能再套一层 shell，
  /// 否则 macOS 本地网络授权对不上真正发起连接的进程。
  private func kotvApplyBundleEnvironment() {
    guard let res = Bundle.main.resourcePath else { return }
    let runtime = (res as NSString).appendingPathComponent("runtime")
    if getenv("KOTV_RUNTIME") == nil, FileManager.default.fileExists(atPath: runtime) {
      setenv("KOTV_RUNTIME", runtime, 0)
    }
    let icd = (res as NSString).appendingPathComponent("vulkan/icd.d/MoltenVK_icd.json")
    guard FileManager.default.fileExists(atPath: icd) else { return }
    if let old = getenv("VK_ICD_FILENAMES") {
      let cur = String(cString: old)
      if !cur.split(separator: ":").contains(Substring(icd)) {
        setenv("VK_ICD_FILENAMES", "\(icd):\(cur)", 1)
      }
    } else {
      setenv("VK_ICD_FILENAMES", icd, 0)
    }
  }

  override func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
    return true
  }

  override func applicationSupportsSecureRestorableState(_ app: NSApplication) -> Bool {
    return true
  }

  override func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
    for window in sender.windows {
      if window.isMiniaturized {
        window.deminiaturize(self)
      }
      window.makeKeyAndOrderFront(self)
    }
    return true
  }
}
