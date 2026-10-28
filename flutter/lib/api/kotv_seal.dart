import 'dart:convert';

import 'package:cryptography/cryptography.dart';

/// 账号密码的加密传输，与引擎 `internal/auth/seal.go` 成对：
/// 临时 X25519 密钥与引擎身份公钥协商，HKDF-SHA256 派生请求、响应两把 AES-256-GCM 密钥，
/// 附加数据为接口路径。只有持有身份私钥的引擎能解开，回复也只有本次请求方能解开。
class KotvSeal {
  KotvSeal._(this._replyKey, this._aad);

  final SecretKey _replyKey;
  final List<int> _aad;

  static final _aes = AesGcm.with256bits();
  static const _macLength = 16;

  /// 用引擎身份公钥 [identity]（base64）加密 [body]。
  /// 返回请求体 `{"sealed":…}`，以及解开回复用的 [KotvSeal]。
  static Future<(Map<String, dynamic>, KotvSeal)> seal(
    String identity,
    String path,
    Map<String, dynamic> body,
  ) async {
    final enginePub = base64Decode(identity);
    final x25519 = X25519();
    final eph = await x25519.newKeyPair();
    final ephPub = (await eph.extractPublicKey()).bytes;
    final shared = await x25519.sharedSecretKey(
      keyPair: eph,
      remotePublicKey: SimplePublicKey(enginePub, type: KeyPairType.x25519),
    );
    final keys = await (await Hkdf(hmac: Hmac.sha256(), outputLength: 64).deriveKey(
      secretKey: shared,
      nonce: [...ephPub, ...enginePub],
      info: utf8.encode('kotv-seal-v1'),
    ))
        .extractBytes();
    final aad = utf8.encode(path);
    final box = await _aes.encrypt(
      utf8.encode(jsonEncode(body)),
      secretKey: SecretKey(keys.sublist(0, 32)),
      aad: aad,
    );
    final request = {
      'sealed': {
        'key': base64Encode(ephPub),
        'iv': base64Encode(box.nonce),
        'data': base64Encode([...box.cipherText, ...box.mac.bytes]),
      },
    };
    return (request, KotvSeal._(SecretKey(keys.sublist(32)), aad));
  }

  /// 解开引擎的加密回复。
  Future<Map<String, dynamic>> open(Map<String, dynamic> reply) async {
    final sealed = reply['sealed'];
    if (sealed is! Map) throw const FormatException('后端没有加密回复');
    final data = base64Decode('${sealed['data']}');
    if (data.length < _macLength) throw const FormatException('后端回复格式错误');
    final plain = await _aes.decrypt(
      SecretBox(
        data.sublist(0, data.length - _macLength),
        nonce: base64Decode('${sealed['iv']}'),
        mac: Mac(data.sublist(data.length - _macLength)),
      ),
      secretKey: _replyKey,
      aad: _aad,
    );
    return Map<String, dynamic>.from(jsonDecode(utf8.decode(plain)) as Map);
  }
}
