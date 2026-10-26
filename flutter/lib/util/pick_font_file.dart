import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';

import 'pick_local_file.dart';

const _fontTypes = XTypeGroup(
  label: '字体',
  extensions: ['ttf', 'otf', 'ttc', 'otc'],
);

/// 选择本地字体文件（Android 仓内浏览器 / SAF；桌面系统对话框）。
Future<String?> kotvPickFontFile(BuildContext context) {
  return kotvPickLocalFile(context, title: '选择字体文件', types: const [_fontTypes]);
}
