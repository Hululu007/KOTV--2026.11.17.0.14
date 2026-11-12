package m3u8

import "strings"

const (
	reasonableGroupLimit = 10
	adBlockSizeRatio     = 0.75
)

// Process 只在节目单带结束标记、且能分出明确少数广告时删段。
// 没有结束标记、证据不足、或少数块太多时，原样返回。
func Process(content string) string {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "#EXT-X-ENDLIST") {
		return content
	}
	lines := splitLines(content)
	ads := findAds(lines)
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

func findAds(lines []string) map[int]struct{} {
	segments := segmentLines(lines)
	// 几乎每片都带 DISCONTINUITY 时，主机「少数派=广告」不可靠（多 CDN / 逐片不连续的正片常见），
	// 只走不连续区块规则；区块也分不清则整单保留。
	if !discontinuitySaturated(lines, len(segments)) {
		if ads := findAdsByLocation(segments); len(ads) > 0 {
			return ads
		}
	}
	return findAdsByDiscontinuity(lines)
}

// discontinuitySaturated：不连续标记数达到切片半数及以上。
func discontinuitySaturated(lines []string, segCount int) bool {
	if segCount < 4 {
		return false
	}
	discs := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "#EXT-X-DISCONTINUITY" {
			discs++
		}
	}
	return discs*2 >= segCount
}

func segmentLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if isSegmentLine(trim) {
			out = append(out, trim)
		}
	}
	return out
}

func isSegmentLine(trim string) bool {
	return trim != "" && !strings.HasPrefix(trim, "#")
}

// findAdsByLocation 按协议加主机，或相对地址的目录分组。
// 组数不在 2 到 10，或没有超过一半的主组时，不按地址删。
func findAdsByLocation(segments []string) map[int]struct{} {
	if len(segments) < 2 {
		return nil
	}
	groups := map[string][]int{}
	for i, seg := range segments {
		key := structuralID(seg)
		groups[key] = append(groups[key], i)
	}
	if len(groups) <= 1 || len(groups) > reasonableGroupLimit {
		return nil
	}
	return minorityIndexes(groups, len(segments))
}

func structuralID(segment string) string {
	if scheme := strings.Index(segment, "://"); scheme >= 0 {
		rest := segment[scheme+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			return segment[:scheme+3+slash]
		}
		return segment
	}
	if slash := strings.LastIndex(segment, "/"); slash >= 0 {
		return segment[:slash]
	}
	return "NO_PATH"
}

func minorityIndexes(groups map[string][]int, total int) map[int]struct{} {
	maxSize := 0
	for _, g := range groups {
		if len(g) > maxSize {
			maxSize = len(g)
		}
	}
	if maxSize*2 <= total {
		return nil
	}
	ads := map[int]struct{}{}
	for _, g := range groups {
		if len(g) < maxSize {
			for _, i := range g {
				ads[i] = struct{}{}
			}
		}
	}
	return ads
}

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
	limit := minorityLimit(totalDurationMinutes(lines))
	if minority > 0 && minority <= limit {
		return ads
	}
	return nil
}

func discontinuityBlocks(lines []string) [][]int {
	var blocks [][]int
	var cur []int
	index := 0
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "#EXT-X-DISCONTINUITY" {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
			}
			cur = nil
			continue
		}
		if isSegmentLine(trim) {
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
	mode := -1
	best := -1
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
		if !strings.HasPrefix(trim, "#EXTINF") {
			continue
		}
		if d, ok := extinfSeconds(trim); ok {
			seconds += d
		}
	}
	return seconds / 60
}

func extinfSeconds(line string) (float64, bool) {
	if len(line) <= len("#EXTINF") || line[len("#EXTINF")] != ':' {
		return 0, false
	}
	body := line[len("#EXTINF")+1:]
	if comma := strings.IndexByte(body, ','); comma >= 0 {
		body = body[:comma]
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return 0, false
	}
	var n float64
	sign := 1.0
	if body[0] == '-' {
		sign = -1
		body = body[1:]
	}
	dot := false
	div := 1.0
	seen := false
	for _, c := range body {
		switch {
		case c >= '0' && c <= '9':
			seen = true
			n = n*10 + float64(c-'0')
			if dot {
				div *= 10
			}
		case c == '.' && !dot:
			dot = true
		default:
			return 0, false
		}
	}
	if !seen {
		return 0, false
	}
	return sign * n / div, true
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
		if isSegmentLine(line) {
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
		if line == "#EXT-X-DISCONTINUITY" {
			prevBoundary := i == 0 || lines[i-1] == "#EXT-X-DISCONTINUITY"
			next := ""
			if i+1 < len(lines) {
				next = lines[i+1]
			}
			nextBoundary := i+1 >= len(lines) || next == "#EXT-X-DISCONTINUITY" || next == "#EXT-X-ENDLIST"
			if prevBoundary || nextBoundary {
				continue
			}
		}
		out = append(out, line)
	}
	return out
}
