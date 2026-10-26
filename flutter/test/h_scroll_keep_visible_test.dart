import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/widgets/h_scroll.dart';

void main() {
  testWidgets('选中项在视口外时拉回，已露出的不动', (tester) async {
    final selected = ValueNotifier<int>(0);
    addTearDown(selected.dispose);
    await tester.pumpWidget(_host(selected, stride: null));
    await tester.pump();

    selected.value = 1;
    await tester.pump();
    await tester.pump();
    expect(_left(tester, '1'), 80);
    expect(tester.getRect(find.byType(HScrollList)).left, 0);

    selected.value = 10;
    for (var i = 0; i < 16; i++) {
      await tester.pump();
    }
    final rect = tester.getRect(find.text('10'));
    final view = tester.getRect(find.byType(HScrollList));
    expect(rect.left, greaterThanOrEqualTo(view.left - 1));
    expect(rect.right, lessThanOrEqualTo(view.right + 1));
  });

  testWidgets('固定步长的选中项一帧内进入视口', (tester) async {
    final selected = ValueNotifier<int>(0);
    addTearDown(selected.dispose);
    await tester.pumpWidget(_host(selected, stride: 80));
    await tester.pump();
    selected.value = 10;
    await tester.pump();
    await tester.pump();
    final rect = tester.getRect(find.text('10'));
    final view = tester.getRect(find.byType(HScrollList));
    expect(rect.left, greaterThanOrEqualTo(view.left - 1));
    expect(rect.right, lessThanOrEqualTo(view.right + 1));
  });
}

Widget _host(ValueNotifier<int> selected, {required double? stride}) {
  return MaterialApp(
    home: Align(
      alignment: Alignment.topLeft,
      child: SizedBox(
        width: 200,
        height: 40,
        child: ValueListenableBuilder<int>(
          valueListenable: selected,
          builder: (_, i, __) => HScrollList(
            scrollToIndex: i,
            itemStride: stride,
            itemCount: 20,
            itemBuilder: (_, n) => SizedBox(
              width: 80,
              height: 40,
              child: Text('$n'),
            ),
          ),
        ),
      ),
    ),
  );
}

double _left(WidgetTester tester, String label) {
  return tester.getTopLeft(find.text(label)).dx;
}
