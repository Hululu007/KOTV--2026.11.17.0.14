import '../api/kotv_api.dart';
import 'engine_api.dart';

Future<KotvApi> _engineApi() => kotvEngineApi();

/// 点播下载：经引擎 /api/v1/download；Flutter 只做入队与任务展示。
class LocalDownload {
  static Future<List<Map<String, dynamic>>> list() async {
    final data = await (await _engineApi()).downloadList();
    final raw = (data['tasks'] as List?) ?? const [];
    return raw
        .whereType<Map>()
        .map((e) => Map<String, dynamic>.from(e))
        .toList();
  }

  /// 某片某线路下已有任务的集序号（引擎 epIndex；删除记录后才可再入队）。
  static Set<int> claimedEpIndexes(
    List<Map<String, dynamic>> tasks, {
    required String site,
    required String vodId,
    required String flag,
  }) {
    final out = <int>{};
    for (final t in tasks) {
      if ('${t['site'] ?? ''}' != site) continue;
      if ('${t['vodId'] ?? ''}' != vodId) continue;
      if ('${t['flag'] ?? ''}' != flag) continue;
      final raw = t['epIndex'];
      if (raw is int) {
        out.add(raw);
      } else {
        final n = int.tryParse('${raw ?? ''}');
        if (n != null) out.add(n);
      }
    }
    return out;
  }

  static Future<Map<String, dynamic>> enqueue({
    required String site,
    required String vodId,
    required String title,
    required String flag,
    String pic = '',
    required List<Map<String, dynamic>> episodes,
  }) async {
    return (await _engineApi()).downloadEnqueue({
      'site': site,
      'vodId': vodId,
      'title': title,
      'flag': flag,
      'pic': pic,
      'episodes': episodes,
    });
  }

  static Future<void> cancel(String id) async {
    await (await _engineApi()).downloadCancel(id);
  }

  static Future<void> pause(String id) async {
    await (await _engineApi()).downloadPause(id);
  }

  static Future<void> resume(String id) async {
    await (await _engineApi()).downloadResume(id);
  }

  static Future<void> remove(String id, {bool deleteFile = false}) async {
    await (await _engineApi()).downloadRemove(id, deleteFile: deleteFile);
  }
}
