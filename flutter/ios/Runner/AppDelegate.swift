import Flutter
import UIKit
import Darwin

@main
@objc class AppDelegate: FlutterAppDelegate {
  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    GeneratedPluginRegistrant.register(with: self)
    let ok = super.application(application, didFinishLaunchingWithOptions: launchOptions)
    if let controller = window?.rootViewController as? FlutterViewController {
      let channel = FlutterMethodChannel(
        name: "kotv_host",
        binaryMessenger: controller.binaryMessenger
      )
      channel.setMethodCallHandler { call, result in
        if call.method == "getMemoryInfo" {
          result(Self.memoryInfo())
        } else {
          result(FlutterMethodNotImplemented)
        }
      }
    }
    return ok
  }

  private static func memoryInfo() -> [String: Int64] {
    let total = Int64(ProcessInfo.processInfo.physicalMemory)
    var stats = vm_statistics64()
    var count = mach_msg_type_number_t(
      MemoryLayout<vm_statistics64_data_t>.stride / MemoryLayout<integer_t>.stride)
    let host = mach_host_self()
    let kr = withUnsafeMutablePointer(to: &stats) {
      $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
        host_statistics64(host, HOST_VM_INFO64, $0, &count)
      }
    }
    var pageSize: vm_size_t = 0
    host_page_size(host, &pageSize)
    let avail: Int64
    if kr == KERN_SUCCESS {
      let freePages = UInt64(stats.free_count + stats.inactive_count + stats.purgeable_count)
      avail = Int64(freePages * UInt64(pageSize))
    } else {
      avail = 0
    }
    return ["totalBytes": total, "availBytes": avail]
  }
}
