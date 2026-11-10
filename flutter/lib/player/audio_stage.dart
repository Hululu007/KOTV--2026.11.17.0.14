import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../widgets/kotv_network_image.dart';
import 'kotv_playback.dart';

/// 无视频尺寸且非纯音频时，Android Hybrid 才需要盖黑防叠影。
bool kotvNeedsBlankSurfaceCover(KotvPlayback pb) =>
    !pb.isAudioOnlyContent && pb.width <= 0 && pb.height <= 0;

/// 纯音频封面/动效叠层：只重建本层，不重建底层 Texture。
Widget kotvAudioStageOverlay({
  required KotvPlayback playback,
  String coverUrl = '',
  String title = '',
}) {
  return ListenableBuilder(
    listenable: playback,
    builder: (context, _) {
      if (!playback.isAudioOnlyContent) return const SizedBox.shrink();
      return IgnorePointer(
        child: KotvAudioStage(
          playing: playback.playing,
          coverUrl: coverUrl,
          title: title,
        ),
      );
    },
  );
}

/// 纯音频时盖在视频槽上：有封面用封面，否则呼吸光晕，避免黑屏。
class KotvAudioStage extends StatefulWidget {
  const KotvAudioStage({
    super.key,
    required this.playing,
    this.coverUrl = '',
    this.title = '',
  });

  final bool playing;
  final String coverUrl;
  final String title;

  @override
  State<KotvAudioStage> createState() => _KotvAudioStageState();
}

class _KotvAudioStageState extends State<KotvAudioStage> with SingleTickerProviderStateMixin {
  late final AnimationController _pulse;

  @override
  void initState() {
    super.initState();
    _pulse = AnimationController(vsync: this, duration: const Duration(milliseconds: 2400));
    if (widget.playing) {
      _pulse.repeat(reverse: true);
    }
  }

  @override
  void didUpdateWidget(covariant KotvAudioStage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.playing == oldWidget.playing) return;
    if (widget.playing) {
      _pulse.repeat(reverse: true);
    } else {
      _pulse.stop();
      _pulse.value = 0.35;
    }
  }

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final cover = widget.coverUrl.trim();
    final title = widget.title.trim();
    return ColoredBox(
      color: const Color(0xFF141018),
      child: Stack(
        fit: StackFit.expand,
        children: [
          AnimatedBuilder(
            animation: _pulse,
            builder: (context, _) {
              final t = widget.playing ? _pulse.value : 0.35;
              return CustomPaint(
                painter: _AudioGlowPainter(t: t),
              );
            },
          ),
          Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                _CoverDisc(coverUrl: cover, playing: widget.playing, pulse: _pulse),
                if (title.isNotEmpty) ...[
                  const SizedBox(height: 20),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 28),
                    child: Text(
                      title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      textAlign: TextAlign.center,
                      style: const TextStyle(
                        color: Color(0xE6FFFFFF),
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                        height: 1.25,
                      ),
                    ),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _CoverDisc extends StatelessWidget {
  const _CoverDisc({
    required this.coverUrl,
    required this.playing,
    required this.pulse,
  });

  final String coverUrl;
  final bool playing;
  final Animation<double> pulse;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: pulse,
      builder: (context, child) {
        final scale = playing ? 1.0 + pulse.value * 0.04 : 1.0;
        return Transform.scale(scale: scale, child: child);
      },
      child: Container(
        width: 168,
        height: 168,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          boxShadow: [
            BoxShadow(
              color: Color.fromRGBO(72, 160, 200, playing ? 0.45 : 0.22),
              blurRadius: playing ? 28 : 16,
              spreadRadius: 2,
            ),
          ],
        ),
        clipBehavior: Clip.antiAlias,
        child: coverUrl.isEmpty
            ? const ColoredBox(
                color: Color(0xFF2A3340),
                child: Icon(Icons.audiotrack_rounded, color: Color(0xCCFFFFFF), size: 56),
              )
            : KotvNetworkImage(
                coverUrl,
                fit: BoxFit.cover,
                errorBuilder: (_, __, ___) => const ColoredBox(
                  color: Color(0xFF2A3340),
                  child: Icon(Icons.audiotrack_rounded, color: Color(0xCCFFFFFF), size: 56),
                ),
              ),
      ),
    );
  }
}

class _AudioGlowPainter extends CustomPainter {
  _AudioGlowPainter({required this.t});

  final double t;

  @override
  void paint(Canvas canvas, Size size) {
    final c = Offset(size.width / 2, size.height / 2);
    final base = math.min(size.width, size.height) * 0.18;
    for (var i = 0; i < 3; i++) {
      final r = base * (1.6 + i * 0.85) * (0.92 + t * 0.16);
      final paint = Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.2
        ..color = Color.fromRGBO(90, 160, 190, (0.22 - i * 0.05).clamp(0.04, 0.22));
      canvas.drawCircle(c, r, paint);
    }
    final wash = Paint()
      ..shader = RadialGradient(
        colors: [
          Color.fromRGBO(70, 130, 160, 0.18 + t * 0.1),
          const Color(0x00141018),
        ],
      ).createShader(Rect.fromCircle(center: c, radius: size.shortestSide * 0.55));
    canvas.drawRect(Offset.zero & size, wash);
  }

  @override
  bool shouldRepaint(covariant _AudioGlowPainter oldDelegate) => oldDelegate.t != t;
}
