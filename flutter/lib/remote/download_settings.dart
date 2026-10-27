/// 下载设置选项（与引擎 download 包规则/并发键一致）。
class DownloadSettings {
  DownloadSettings._();

  static const nameRuleDefault = 'title_flag_ep';

  /// (界面文案, 存盘 ID)
  static const nameRuleOptions = <(String, String)>[
    ('片名/线路_集数', 'title_flag_ep'),
    ('片名/集数', 'title_ep'),
    ('线路_集数（不分文件夹）', 'flag_ep'),
    ('片名_集数（不分文件夹）', 'title_ep_flat'),
  ];

  static const classifyOptions = <(String, String)>[
    ('按片名建子目录', 'true'),
    ('全部放在下载根目录', 'false'),
  ];

  static const concurrencyOptions = <(String, String)>[
    ('1 个任务', '1'),
    ('2 个任务', '2'),
    ('3 个任务', '3'),
    ('4 个任务', '4'),
    ('5 个任务', '5'),
  ];

  static const threadOptions = <(String, String)>[
    ('1 线程', '1'),
    ('2 线程', '2'),
    ('4 线程', '4'),
    ('8 线程', '8'),
  ];

  static const filterOptions = <(String, String)>[
    ('全部', 'all'),
    ('下载中', 'running'),
    ('排队中', 'queued'),
    ('已暂停', 'paused'),
    ('已完成', 'done'),
    ('已失败', 'error'),
  ];

  static String nameRuleLabel(String id) {
    final key = normalizeNameRule(id);
    for (final e in nameRuleOptions) {
      if (e.$2 == key) return e.$1;
    }
    return nameRuleOptions.first.$1;
  }

  static String normalizeNameRule(String raw) {
    final v = raw.trim();
    if (v.isEmpty) return nameRuleDefault;
    for (final e in nameRuleOptions) {
      if (e.$2 == v) return v;
    }
    if (v.contains('{title}/{flag}_{ep}')) return 'title_flag_ep';
    if (v.contains('{title}/{ep}')) return 'title_ep';
    if (v.contains('{title}_{ep}')) return 'title_ep_flat';
    if (v.contains('{flag}_{ep}')) return 'flag_ep';
    return nameRuleDefault;
  }

  static String classifyLabel(String raw) {
    final on = raw.trim().isEmpty || raw == 'true' || raw == '1';
    return on ? classifyOptions.first.$1 : classifyOptions.last.$1;
  }

  static String concurrencyLabel(String raw) {
    final v = _clampInt(raw, 2, 1, 5).toString();
    for (final e in concurrencyOptions) {
      if (e.$2 == v) return e.$1;
    }
    return '$v 个任务';
  }

  static String threadsLabel(String raw) {
    final n = _clampInt(raw, 4, 1, 8);
    final v = n == 3 || n == 5 || n == 6 || n == 7 ? '4' : n.toString();
    for (final e in threadOptions) {
      if (e.$2 == v) return e.$1;
    }
    return '$v 线程';
  }

  static String normalizeConcurrency(String raw) =>
      _clampInt(raw, 2, 1, 5).toString();

  static String normalizeThreads(String raw) {
    final n = _clampInt(raw, 4, 1, 8);
    if (n == 1 || n == 2 || n == 4 || n == 8) return '$n';
    return '4';
  }

  static int _clampInt(String raw, int def, int min, int max) {
    final n = int.tryParse(raw.trim()) ?? def;
    if (n < min) return min;
    if (n > max) return max;
    return n;
  }

  static String dirLabel(String dir) {
    final d = dir.trim();
    if (d.isEmpty) return '引擎默认 Downloads';
    if (d.length <= 28) return d;
    return '…${d.substring(d.length - 26)}';
  }

  static bool matchFilter(String status, String filter) {
    switch (filter) {
      case 'all':
        return true;
      case 'running':
        return status == 'running';
      case 'queued':
        return status == 'queued';
      case 'paused':
        return status == 'paused';
      case 'done':
        return status == 'done';
      case 'error':
        return status == 'error' || status == 'cancelled';
      default:
        return true;
    }
  }

  static String taskLabel(Map<String, dynamic> t) {
    final title = '${t['title'] ?? ''}'.trim();
    final ep = '${t['epName'] ?? ''}'.trim();
    final flag = '${t['flag'] ?? ''}'.trim();
    final parts = <String>[
      if (title.isNotEmpty) title,
      if (flag.isNotEmpty) flag,
      if (ep.isNotEmpty) ep,
    ];
    return parts.isEmpty ? '${t['id'] ?? ''}' : parts.join(' · ');
  }

  /// 任务列表状态行文案（进度/错误优先用引擎 message、error）。
  static String statusLabel(Map<String, dynamic> t) {
    final st = '${t['status'] ?? ''}';
    final msg = '${t['message'] ?? ''}'.trim();
    final err = '${t['error'] ?? ''}'.trim();
    switch (st) {
      case 'queued':
        return msg.isEmpty ? '等待中' : msg;
      case 'running':
        final p =
            (t['progress'] is num) ? (t['progress'] as num).toDouble() : 0.0;
        final pct = (p * 100).clamp(0, 100).toStringAsFixed(0);
        return msg.isEmpty ? '下载中 $pct%' : '$msg · $pct%';
      case 'paused':
        return msg.isEmpty ? '已暂停' : msg;
      case 'done':
        return '已完成';
      case 'cancelled':
        return '已取消';
      case 'error':
        return err.isNotEmpty ? err : (msg.isEmpty ? '失败' : msg);
      default:
        return st;
    }
  }
}
