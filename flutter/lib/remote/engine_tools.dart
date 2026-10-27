// 引擎 tools 结果文案（设置页 / 我的 共用）。

String _toolsMessage(Map<String, dynamic> data) {
  final msg = '${data['message'] ?? ''}'.trim();
  return msg.isEmpty ? '检查完成' : msg;
}

String kotvFormatCheckUpdateResult(Map<String, dynamic> data) =>
    _toolsMessage(data);

String kotvFormatCheckSpiderResult(Map<String, dynamic> data) {
  final msg = '${data['message'] ?? ''}'.trim();
  final results =
      ((data['results'] as List?) ?? []).whereType<Map>().toList();
  if (results.isEmpty) return _toolsMessage(data);
  final lines = results.take(20).map((e) {
    final ok = e['ok'] == true;
    final name = '${e['name'] ?? e['key'] ?? ''}'.trim();
    final err = '${e['message'] ?? ''}'.trim();
    return '${ok ? '✓' : '✗'} $name${ok || err.isEmpty ? '' : ' — $err'}';
  });
  if (msg.isEmpty) return lines.join('\n');
  return '$msg\n\n${lines.join('\n')}';
}
