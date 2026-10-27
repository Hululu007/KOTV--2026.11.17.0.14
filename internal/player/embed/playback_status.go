//go:build cgo

package embed

// EmbedPlaybackSnapshot 页内播放状态（供 /api/v1/player GET；不含 JPEG 帧）。
func EmbedPlaybackSnapshot() map[string]any {
	eng := Active()
	out := map[string]any{
		"embedActive": true,
		"playing":     eng.IsPlaying(),
		"positionMs":  eng.PositionMs(),
		"durationMs":  eng.DurationMs(),
		"volume":      eng.Volume(),
		"ended":       eng.PlaybackEnded(),
	}
	if enh, ok := eng.(Enhanced); ok {
		w, h := enh.VideoSize()
		out["videoW"] = w
		out["videoH"] = h
		out["speed"] = enh.Speed()
	}
	return out
}
