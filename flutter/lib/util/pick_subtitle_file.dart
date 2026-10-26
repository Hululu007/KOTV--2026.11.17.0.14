import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';

import 'pick_local_file.dart';

const _subtitleTypes = XTypeGroup(
  label: '字幕',
  extensions: ['srt', 'ass', 'ssa', 'vtt', 'sub', 'idx', 'sup', 'txt'],
);

/// 选择本地字幕文件：Android 复用仓内文件浏览器 / SAF；桌面走系统对话框。
Future<String?> kotvPickSubtitleFile(BuildContext context) {
  return kotvPickLocalFile(context, title: '选择字幕文件', types: const [_subtitleTypes]);
}
