import 'dart:async';
import 'dart:js_interop';

import 'package:web/web.dart' as web;

class KotvUiSocket {
  KotvUiSocket(this._ws, this.frames);

  final web.WebSocket _ws;
  final Stream<String> frames;

  void send(String text) {
    _ws.send(text.toJS);
  }

  Future<void> close() async {
    _ws.close();
  }
}

Future<KotvUiSocket> kotvOpenUiSocket(String url, Map<String, String> headers) {
  final opened = Completer<KotvUiSocket>();
  final frames = StreamController<String>();
  final ws = web.WebSocket(url);
  ws.onopen = ((web.Event _) {
    if (!opened.isCompleted) opened.complete(KotvUiSocket(ws, frames.stream));
  }).toJS;
  ws.onmessage = ((web.Event event) {
    final data = (event as web.MessageEvent).data;
    if (data != null && data.isA<JSString>()) {
      frames.add((data as JSString).toDart);
    }
  }).toJS;
  ws.onerror = ((web.Event _) {
    if (!opened.isCompleted) opened.completeError(StateError('ui socket'));
  }).toJS;
  ws.onclose = ((web.Event _) {
    if (!opened.isCompleted) opened.completeError(StateError('ui socket'));
    unawaited(frames.close());
  }).toJS;
  return opened.future;
}
