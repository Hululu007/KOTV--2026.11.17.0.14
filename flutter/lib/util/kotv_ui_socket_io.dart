import 'dart:async';
import 'dart:io';

class KotvUiSocket {
  KotvUiSocket(this._ws);

  final WebSocket _ws;

  Stream<String> get frames => _ws.where((event) => event is String).cast<String>();

  void send(String text) {
    _ws.add(text);
  }

  Future<void> close() => _ws.close();
}

Future<KotvUiSocket> kotvOpenUiSocket(String url, Map<String, String> headers) async {
  final ws = await WebSocket.connect(url, headers: headers);
  return KotvUiSocket(ws);
}
