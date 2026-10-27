import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/models.dart';
import '../remote/remote_bridge.dart';
import 'library_shelf.dart';

class HistoryScreen extends ConsumerStatefulWidget {
  const HistoryScreen({super.key});

  @override
  ConsumerState<HistoryScreen> createState() => _HistoryScreenState();
}

class _HistoryScreenState extends ConsumerState<HistoryScreen> {
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
    final list = await LocalHistory.list();
    if (!mounted) return;
    setState(() {
      _items = list;
      _loading = false;
    });
  }

  @override
  Widget build(BuildContext context) {
    return LibraryShelf(
      title: '历史',
      items: _items,
      loading: _loading,
      deleting: _deleting,
      emptyText: '暂无历史记录',
      clearShort: '清空',
      clearFull: '清空全部',
      onToggleDelete: () => setState(() => _deleting = !_deleting),
      onClear: () async {
        await LocalHistory.clear();
        await _reload();
      },
      onRemove: (it) async {
        await LocalHistory.remove(it);
        await _reload();
      },
      onRefresh: _reload,
    );
  }
}
