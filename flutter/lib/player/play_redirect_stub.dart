import 'play_redirect_types.dart';

Future<KotvFollowedPlay> kotvFollowPlayUrl(
  String url,
  Map<String, String> headers,
) async {
  return KotvFollowedPlay(url: url, headers: headers, moved: false);
}
