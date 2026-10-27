import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/models.dart';
import '../theme/kotv_palette.dart';
import '../theme/layout_scale.dart';
import '../vod/grid_style.dart';
import '../vod/vod_open.dart';
import '../widgets/chrome.dart';
import '../widgets/poster_card.dart';
import 'shell.dart';

typedef LibraryShelfExtras = List<Widget> Function({
  required bool compact,
  required bool land,
  required double pillH,
  required double pillFs,
  required double gap,
});

/// 收藏、历史共用的海报库：顶栏、删除模式和空列表。
class LibraryShelf extends ConsumerWidget {
  const LibraryShelf({
    super.key,
    required this.title,
    required this.items,
    required this.loading,
    required this.deleting,
    required this.emptyText,
    required this.clearShort,
    required this.clearFull,
    required this.onToggleDelete,
    required this.onClear,
    required this.onRemove,
    this.extraTrailing,
    this.gridPadding,
  });

  final String title;
  final List<VodItem> items;
  final bool loading;
  final bool deleting;
  final String emptyText;
  final String clearShort;
  final String clearFull;
  final VoidCallback onToggleDelete;
  final Future<void> Function() onClear;
  final Future<void> Function(VodItem item) onRemove;
  final LibraryShelfExtras? extraTrailing;
  final EdgeInsets? gridPadding;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final compact = KotvLayout.isCompact(context) || KotvLayout.useBottomNav(context);
    final land = KotvLayout.isLandscapeCompact(context);
    final pillH = land ? 30.0 : (compact ? 32.0 : 40.0);
    final pillFs = land ? 12.0 : (compact ? 13.0 : 15.0);
    final delW = land ? 72.0 : (compact ? 88.0 : 120.0);
    final clearW = land ? 72.0 : (compact ? 88.0 : 120.0);
    final gap = land ? 6.0 : (compact ? 6.0 : 8.0);
    final p = KotvPalette.of(context);
    final extras = extraTrailing?.call(
          compact: compact,
          land: land,
          pillH: pillH,
          pillFs: pillFs,
          gap: gap,
        ) ??
        const <Widget>[];
    return Column(
      children: [
        LibraryTopBar(
          onBack: () => kotvPageBack(ref),
          onSearch: () => goKotvPage(ref, KotvPage.search),
          onProfile: () => goKotvPage(ref, KotvPage.profile),
          onNews: () => showAppNews(context, remoteHint(ref)),
          title: title,
          trailing: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              AppPill(
                label: deleting ? '完成' : (compact ? '删除' : '删除记录'),
                width: delW,
                height: pillH,
                fontSize: pillFs,
                selected: deleting,
                onTap: onToggleDelete,
              ),
              SizedBox(width: gap),
              AppPill(
                label: compact ? clearShort : clearFull,
                width: clearW,
                height: pillH,
                fontSize: pillFs,
                onTap: () async {
                  await onClear();
                },
              ),
              ...extras,
            ],
          ),
        ),
        if (deleting)
          Padding(
            padding: const EdgeInsets.only(bottom: 4),
            child: Text('删除模式：点击海报即可移除', style: TextStyle(color: p.primary, fontSize: 14)),
          ),
        Expanded(
          child: loading
              ? Center(child: CircularProgressIndicator(color: p.primary))
              : items.isEmpty
                  ? Center(child: Text(emptyText, style: TextStyle(color: p.muted, fontSize: 18)))
                  : PosterFlow(
                      items: items,
                      size: kotvWatchPosterSize(ref),
                      style: kotvGridStyleOfItems(items),
                      padding: gridPadding,
                      onOpen: (it) async {
                        if (deleting) {
                          await onRemove(it);
                          return;
                        }
                        if (!context.mounted) return;
                        await openVodItem(
                          context,
                          ref,
                          it,
                          site: it.site,
                          searchIfIndex: false,
                        );
                      },
                    ),
        ),
      ],
    );
  }
}
