/// 下载设置选项（与引擎 [internal/download] 命名 ID 一致）。
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
    // 兼容旧版手写模板
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

  static String dirLabel(String dir) {
    final d = dir.trim();
    if (d.isEmpty) return '系统默认 Downloads';
    if (d.length <= 28) return d;
    return '…${d.substring(d.length - 26)}';
  }
}
