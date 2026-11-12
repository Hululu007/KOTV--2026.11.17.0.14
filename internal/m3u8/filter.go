package m3u8

import (
	"strconv"
	"strings"
)

// 不连续区块相对众数块长度的比例：更短的视为广告候选。
const adBlockSizeRatio = 0.75

// Process 仅在带 #EXT-X-ENDLIST 的点播节目单上，按不连续区块长度删明确少数广告。
// 证据不足（无结束标记、切不出众数块、候选过多）时原样返回。
// 不按「少数主机」删段：双 CDN / 逐片 DISCONTINUITY 的正片会被误伤。
func Process(content string) string {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "#EXT-X-ENDLIST") {
		return content
	}
	lines := splitLines(content)
	ads := findAdsByDiscontinuity(lines)
	if len(ads) == 0 {
		return content
	}
	return rebuild(lines, ads)
}

func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.Split(content, "\n")
}

func isSegmentURI(trim string) bool {
	return trim != "" && !strings.HasPrefix(trim, "#")
}

// findAdsByDiscontinuity 以 DISCONTINUITY 分块；比众数块明显更短、且块数不超过片长上限的，当作广告。
// 末块不参与判定（片尾短块常见）。逐片都断开时众数块长为 1，不会误删。
func findAdsByDiscontinuity(lines []string) map[int]struct{} {
	blocks := discontinuityBlocks(lines)
	if len(blocks) < 2 {
		return nil
	}
	analysis := blocks[:len(blocks)-1]
	modeSize := modeBlockSize(analysis)
	if modeSize <= 0 {
		return nil
	}
	threshold := float64(modeSize) * adBlockSizeRatio
	ads := map[int]struct{}{}
	minority := 0
	for _, block := range analysis {
		if float64(len(block)) < threshold {
			minority++
			for _, i := range block {
				ads[i] = struct{}{}
			}
		}
	}
	if minority == 0 || minority > minorityLimit(totalDurationMinutes(lines)) {
		return nil
	}
	return ads
}

func discontinuityBlocks(lines []string) [][]int {
	var blocks [][]int
	var cur []int
	index := 0
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		switch {
		case trim == "#EXT-X-DISCONTINUITY":
			if len(cur) > 0 {
				blocks = append(blocks, cur)
			}
			cur = nil
		case isSegmentURI(trim):
			cur = append(cur, index)
			index++
		}
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	return blocks
}

func modeBlockSize(blocks [][]int) int {
	freq := map[int]int{}
	for _, block := range blocks {
		freq[len(block)]++
	}
	mode, best := -1, -1
	for size, n := range freq {
		if n > best || (n == best && size > mode) {
			best = n
			mode = size
		}
	}
	return mode
}

func totalDurationMinutes(lines []string) float64 {
	var seconds float64
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if d, ok := extinfSeconds(trim); ok {
			seconds += d
		}
	}
	return seconds / 60
}

func extinfSeconds(line string) (float64, bool) {
	if !strings.HasPrefix(line, "#EXTINF:") {
		return 0, false
	}
	body := strings.TrimSpace(line[len("#EXTINF:"):])
	if i := strings.IndexByte(body, ','); i >= 0 {
		body = body[:i]
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(body, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func minorityLimit(minutes float64) int {
	switch {
	case minutes <= 30:
		return 3
	case minutes <= 60:
		return 4
	case minutes <= 90:
		return 5
	default:
		return 6
	}
}

func rebuild(lines []string, ads map[int]struct{}) string {
	kept := removeAdSegments(lines, ads)
	kept = removeOrphanedDiscontinuity(kept)
	var b strings.Builder
	for _, line := range kept {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func removeAdSegments(lines []string, ads map[int]struct{}) []string {
	out := make([]string, 0, len(lines))
	skipping := false
	index := 0
	for _, source := range lines {
		line := strings.TrimSpace(source)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF") {
			if _, ad := ads[index]; ad {
				skipping = true
				continue
			}
		}
		if isSegmentURI(line) {
			_, ad := ads[index]
			index++
			if skipping || ad {
				skipping = false
				continue
			}
		} else if skipping {
			continue
		}
		out = append(out, line)
	}
	return out
}

func removeOrphanedDiscontinuity(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if line != "#EXT-X-DISCONTINUITY" {
			out = append(out, line)
			continue
		}
		prevBoundary := i == 0 || lines[i-1] == "#EXT-X-DISCONTINUITY"
		next := ""
		if i+1 < len(lines) {
			next = lines[i+1]
		}
		nextBoundary := i+1 >= len(lines) || next == "#EXT-X-DISCONTINUITY" || next == "#EXT-X-ENDLIST"
		if prevBoundary || nextBoundary {
			continue
		}
		out = append(out, line)
	}
	return out
}
