class KotvFollowedPlay {
  const KotvFollowedPlay({
    required this.url,
    required this.headers,
    required this.moved,
  });

  final String url;
  final Map<String, String> headers;
  final bool moved;
}
