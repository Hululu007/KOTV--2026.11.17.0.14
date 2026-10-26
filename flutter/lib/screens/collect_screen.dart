import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/models.dart';
import '../providers.dart';
import '../remote/local_collect.dart';
import 'library_shelf.dart';

class CollectScreen extends ConsumerStatefulWidget {
  const CollectScreen({super.key});

  @override
  ConsumerState<CollectScreen> createState() => _CollectScreenState();
}

class _CollectScreenState extends ConsumerState<CollectScreen> {
  List<VodItem> _items = [];
  bool _loading = true;
  bool _deleting = false;

  @override
  void initState() {
    super.initState();
    _reload();
  }

  Future<void> _reload() async {
    setState(() => _loading = true);
    var currentSource = '';
    try {
      final cfg = await ref.read(apiProvider).getConfig();
      currentSource = '${cfg['source'] ?? ''}'.trim();
    } catch (_) {}
    final list = await LocalCollect.list();
    final filtered = list.where((e) {
      final cs = e.configSource.trim();
      return cs.isEmpty || cs == currentSource;
    }).toList();
    if (!mounted) return;
    setState(() {
      _items = filtered;
      _loading = false;
    });
  }

  @override
  Widget build(BuildContext context) {
    return LibraryShelf(
      title: '收藏',
      items: _items,
      loading: _loading,
      deleting: _deleting,
      emptyText: '暂无收藏',
      clearShort: '清空',
      clearFull: '清空收藏',
      onToggleDelete: () => setState(() => _deleting = !_deleting),
      onClear: () async {
        await LocalCollect.clear();
        await _reload();
      },
      onRemove: (it) async {
        await LocalCollect.remove(it);
        await _reload();
      },
      gridPadding: const EdgeInsets.fromLTRB(60, 10, 60, 24),
    );
  }
}
