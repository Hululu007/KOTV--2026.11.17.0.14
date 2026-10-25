import Flutter
import UIKit

class SceneDelegate: FlutterSceneDelegate {}

/// 故事板里的根控制器。iOS 26 上根视图的 safeAreaInsets 会是 0，
/// 顶栏就画进状态栏，左右按钮的点击被系统吃掉。
class KotvFlutterViewController: FlutterViewController {
  /// 上次已经写给 Flutter 的顶部间距。全屏隐藏状态栏后再回来时，数值可能没变，
  /// 但视口仍记着 0，必须再触发一次 viewSafeAreaInsetsDidChange。
  private var pokedTop: CGFloat = -1

  override func viewDidLayoutSubviews() {
    super.viewDidLayoutSubviews()
    guard let window = view.window else { return }
    let win = window.safeAreaInsets
    let bar = window.windowScene?.statusBarManager?.statusBarFrame.height ?? 0
    let neededTop = max(win.top, bar)
    let neededBottom = win.bottom
    let extra = additionalSafeAreaInsets
    let systemTop = view.safeAreaInsets.top - extra.top
    let systemBottom = view.safeAreaInsets.bottom - extra.bottom
    var next = extra
    let targetTop = max(0, neededTop - systemTop)
    if neededTop < 1 {
      pokedTop = -1
    } else if abs(extra.top - targetTop) > 0.5 {
      next.top = targetTop
      pokedTop = neededTop
    } else if abs(neededTop - pokedTop) > 0.5 {
      next.top = targetTop + 0.01
      pokedTop = neededTop
    }
    if systemBottom + 0.5 < neededBottom {
      next.bottom = neededBottom - systemBottom
    }
    if next != additionalSafeAreaInsets {
      additionalSafeAreaInsets = next
    }
  }
}
