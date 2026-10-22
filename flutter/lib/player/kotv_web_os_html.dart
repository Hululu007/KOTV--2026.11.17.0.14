import 'dart:ui_web' as ui_web;

String kotvWebOperatingSystem() {
  switch (ui_web.browser.operatingSystem) {
    case ui_web.OperatingSystem.android:
      return 'android';
    case ui_web.OperatingSystem.iOs:
      return 'ios';
    case ui_web.OperatingSystem.windows:
      return 'windows';
    case ui_web.OperatingSystem.macOs:
      return 'macos';
    case ui_web.OperatingSystem.linux:
      return 'linux';
    case ui_web.OperatingSystem.unknown:
      return '';
  }
}
