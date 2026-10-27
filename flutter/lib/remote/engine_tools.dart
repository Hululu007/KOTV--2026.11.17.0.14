// 引擎 tools 结果文案（设置页 / 我的 共用）。

String kotvFormatCheckUpdateResult(Map<String, dynamic> data) {
  final msg = '${data['message'] ?? ''}'.trim();
  return msg.isEmpty ? '检查完成' : msg;
}
