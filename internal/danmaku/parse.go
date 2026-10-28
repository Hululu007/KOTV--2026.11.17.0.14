package danmaku

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 弹幕模式（B 站 p 属性第二段）。
const (
	ModeScroll     = 1
	ModeBottom     = 4
	ModeTop        = 5
	ModeReverse    = 6
	ModePositioned = 7
)

// 弹幕池（B 站 p 属性第六段）：普通 / 字幕 / 特殊。
const (
	PoolNormal   = 0
	PoolSubtitle = 1
	PoolSpecial  = 2
)

const (
	sniffLength  = 512
	defaultColor = 0xFFFFFF
	defaultSize  = 25 // B 站中号；前端按 18 以下小号、36 以上大号缩放
	// 定位弹幕坐标若是像素值，按 B 站旧播放器尺寸归一化。
	biliPlayerWidth  = 672.0
	biliPlayerHeight = 438.0
)

var errUnsupported = errors.New("不支持的弹幕格式")

// parser 嗅探正文开头判断格式，命中后解析全文。顺序即优先级。
type parser struct {
	sniff func(head string, data []byte) bool
	parse func(data []byte) ([]Item, error)
}

var parsers = []parser{
	{sniffBili, parseBili},
	{sniffTxt, parseTxt},
	{func(h string, _ []byte) bool { return strings.Contains(h, `"barrage_list"`) }, parseQQ},
	{func(h string, _ []byte) bool {
		return strings.Contains(h, `"playat"`) || strings.Contains(h, `"propertis"`)
	}, parseYouku},
	{func(h string, _ []byte) bool {
		return strings.HasPrefix(strings.TrimSpace(h), "{") && strings.Contains(h, `"items"`)
	}, parseMGTV},
	{func(_ string, d []byte) bool { return sniffIQIYI(d) }, parseIQIYI},
}

// Parse 识别格式并解析：B 站 XML、[p]文本、腾讯/优酷/芒果 JSON、爱奇艺 protobuf。结果按时间排序。
func Parse(data []byte) ([]Item, error) {
	if len(data) == 0 {
		return nil, nil
	}
	head := sniffHead(data)
	for _, p := range parsers {
		if p.sniff(head, data) {
			items, err := p.parse(data)
			if err != nil {
				return nil, err
			}
			sortByTime(items)
			return items, nil
		}
	}
	return nil, errUnsupported
}

func sniffHead(data []byte) string {
	if len(data) > sniffLength {
		data = data[:sniffLength]
	}
	return stripBOM(data)
}

func sortByTime(items []Item) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Time < items[j].Time })
}

func capItems(items []Item) []Item {
	if len(items) > itemsMax {
		return items[:itemsMax]
	}
	return items
}

// ---- B 站 XML ----

func sniffBili(head string, _ []byte) bool {
	t := strings.TrimSpace(head)
	if strings.HasPrefix(t, "<?xml") {
		if i := strings.Index(t, "?>"); i >= 0 {
			t = strings.TrimSpace(t[i+2:])
		}
	}
	return strings.HasPrefix(t, "<i>") || strings.HasPrefix(t, "<i ")
}

func parseBili(data []byte) ([]Item, error) {
	dec := xml.NewDecoder(strings.NewReader(stripBOM(data)))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var out []Item
	for len(out) < itemsMax {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "d" {
			continue
		}
		var d struct {
			P    string `xml:"p,attr"`
			Text string `xml:",chardata"`
		}
		if err := dec.DecodeElement(&d, &se); err != nil {
			return nil, err
		}
		if d.P == "" || d.Text == "" {
			continue
		}
		if it, ok := parsePAttr(d.P, d.Text); ok {
			out = append(out, it)
		}
	}
	return out, nil
}

// parsePAttr 「时间,模式,字号,颜色[,时间戳,弹幕池,用户,行号]」；前四段缺一或不是数字就丢弃。
// 模式 7 的正文是 JSON 数组 [x,y,透明度,时长,文字,…]。
func parsePAttr(p, text string) (Item, bool) {
	parts := strings.Split(p, ",")
	if len(parts) < 4 {
		return Item{}, false
	}
	t, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	mode, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	size, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	color, err4 := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return Item{}, false
	}
	it := Item{Time: t, Mode: mode, Size: size, Color: int(color & 0xFFFFFF), Content: text}
	if mode == ModePositioned {
		return positioned(it)
	}
	switch mode {
	case 1, 2, 3:
		it.Mode = ModeScroll
	case ModeBottom, ModeTop, ModeReverse:
	default:
		return Item{}, false
	}
	if len(parts) >= 6 {
		if pool, err := strconv.Atoi(strings.TrimSpace(parts[5])); err == nil && (pool == PoolSubtitle || pool == PoolSpecial) {
			it.Pool = pool
		}
	}
	return it, true
}

func positioned(it Item) (Item, bool) {
	var values []any
	if err := json.Unmarshal([]byte(it.Content), &values); err != nil || len(values) <= 4 {
		return Item{}, false
	}
	text, _ := values[4].(string)
	if text == "" {
		return Item{}, false
	}
	num := func(i int, def float64) float64 {
		switch v := values[i].(type) {
		case float64:
			return v
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
		return def
	}
	norm := func(v, size float64) float64 {
		if v >= 0 && v <= 1 {
			return v
		}
		return v / size
	}
	it.Content = text
	it.Pool = PoolSpecial
	it.X = norm(num(0, biliPlayerWidth/2), biliPlayerWidth)
	it.Y = norm(num(1, biliPlayerHeight/2), biliPlayerHeight)
	if d := num(3, 0); d > 0 {
		it.DurationMs = int64(d * 1000)
	}
	return it, true
}

// ---- [p]文本 ----

var (
	txtLineRe = regexp.MustCompile(`\[([^\]]+)\](.*)`)
	// 文本格式首行以「[数字」开头；JSON 的 [{…}] 不算。
	txtSniffRe = regexp.MustCompile(`^\[[\d.+-]`)
)

func sniffTxt(head string, _ []byte) bool {
	for _, line := range strings.Split(head, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return txtSniffRe.MatchString(t) && txtLineRe.MatchString(t)
		}
	}
	return false
}

// parseTxtLine 解析一行「[时间,模式,字号,颜色]正文」。
func parseTxtLine(line string) (Item, bool) {
	m := txtLineRe.FindStringSubmatch(line)
	if m == nil || m[2] == "" {
		return Item{}, false
	}
	return parsePAttr(m[1], m[2])
}

func parseTxt(data []byte) ([]Item, error) {
	var out []Item
	for _, line := range strings.Split(stripBOM(data), "\n") {
		if len(out) >= itemsMax {
			break
		}
		if it, ok := parseTxtLine(strings.TrimRight(line, "\r")); ok {
			out = append(out, it)
		}
	}
	return out, nil
}

func stripBOM(data []byte) string {
	return string(bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF")))
}

// ---- 腾讯 / 优酷 / 芒果 JSON ----

func parseQQ(data []byte) ([]Item, error) {
	var doc struct {
		List []struct {
			TimeOffset   string `json:"time_offset"`
			Content      string `json:"content"`
			ContentStyle string `json:"content_style"`
		} `json:"barrage_list"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var out []Item
	for _, b := range doc.List {
		ms, err := strconv.ParseInt(b.TimeOffset, 10, 64)
		if err != nil || b.Content == "" {
			continue
		}
		color := defaultColor
		if b.ContentStyle != "" {
			var style struct {
				Color          string   `json:"color"`
				GradientColors []string `json:"gradient_colors"`
			}
			if json.Unmarshal([]byte(b.ContentStyle), &style) == nil {
				color = hexColor(style.Color, color)
				if g := hexColors(style.GradientColors); len(g) > 0 {
					color = g[0]
				}
			}
		}
		out = append(out, scrollItem(ms, b.Content, color))
	}
	return capItems(out), nil
}

func parseYouku(data []byte) ([]Item, error) {
	var doc struct {
		Data struct {
			Result []struct {
				Playat    float64 `json:"playat"`
				Content   string  `json:"content"`
				Propertis string  `json:"propertis"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var out []Item
	for _, r := range doc.Data.Result {
		if r.Content == "" {
			continue
		}
		color := defaultColor
		if r.Propertis != "" {
			var props struct {
				Color          *float64  `json:"color"`
				GradientColors []float64 `json:"gradientColors"`
			}
			if json.Unmarshal([]byte(r.Propertis), &props) == nil {
				if props.Color != nil {
					color = int(int64(math.Round(*props.Color)) & 0xFFFFFF)
				}
				if len(props.GradientColors) >= 2 {
					color = int(int64(math.Round(props.GradientColors[0])) & 0xFFFFFF)
				}
			}
		}
		out = append(out, scrollItem(int64(r.Playat), r.Content, color))
	}
	return capItems(out), nil
}

func parseMGTV(data []byte) ([]Item, error) {
	type rgb struct {
		R *float64 `json:"r"`
		G *float64 `json:"g"`
		B *float64 `json:"b"`
	}
	var doc struct {
		Data struct {
			Items []struct {
				Time    float64 `json:"time"`
				Content string  `json:"content"`
				V2Color *struct {
					Left  *rgb `json:"color_left"`
					Right *rgb `json:"color_right"`
				} `json:"v2_color"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	pack := func(c *rgb) int {
		ch := func(v *float64) int {
			if v == nil {
				return 255
			}
			return int(math.Round(*v)) & 0xFF
		}
		return ch(c.R)<<16 | ch(c.G)<<8 | ch(c.B)
	}
	var out []Item
	for _, it := range doc.Data.Items {
		if it.Content == "" {
			continue
		}
		color := defaultColor
		if c := it.V2Color; c != nil && c.Left != nil {
			color = pack(c.Left)
			if c.Right != nil {
				l, r := color, pack(c.Right)
				avg := func(shift int) int { return (((l >> shift) & 0xFF) + ((r >> shift) & 0xFF) + 1) / 2 }
				color = avg(16)<<16 | avg(8)<<8 | avg(0)
			}
		}
		out = append(out, scrollItem(int64(it.Time), it.Content, color))
	}
	return capItems(out), nil
}

func scrollItem(ms int64, text string, color int) Item {
	return Item{Time: float64(ms) / 1000, Mode: ModeScroll, Size: defaultSize, Color: color, Content: text}
}

func hexColor(hex string, def int) int {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return def
	}
	v, err := strconv.ParseInt(h, 16, 64)
	if err != nil {
		return def
	}
	return int(v)
}

// hexColors 渐变色；有效色少于两个视为没有渐变。
func hexColors(list []string) []int {
	var out []int
	for _, h := range list {
		if c := hexColor(h, -1); c >= 0 {
			out = append(out, c)
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// ---- 爱奇艺 protobuf ----
// 结构：字段 6 = 弹幕条目（重复），条目内字段 2 = 弹幕；弹幕内 2 = 正文，6 = 秒（字符串），8 = 颜色。

const (
	wireVarint = 0
	wire64     = 1
	wireLen    = 2
	wire32     = 5
)

func sniffIQIYI(data []byte) bool {
	if len(data) < 3 || data[0] != 6<<3|wireLen {
		return false
	}
	n, size := readVarint(data[1:])
	if size <= 0 || n == 0 || 1+size >= len(data) {
		return false
	}
	return data[1+size] == 2<<3|wireLen
}

func parseIQIYI(data []byte) ([]Item, error) {
	var out []Item
	walkProto(data, func(field int, entry []byte) {
		if field != 6 {
			return
		}
		walkProto(entry, func(field int, bullet []byte) {
			if field != 2 || len(out) >= itemsMax {
				return
			}
			if it, ok := iqiyiBullet(bullet); ok {
				out = append(out, it)
			}
		})
	})
	return out, nil
}

func iqiyiBullet(b []byte) (Item, bool) {
	var content, timeStr, colorStr string
	walkProto(b, func(field int, v []byte) {
		switch field {
		case 2:
			content = string(v)
		case 6:
			timeStr = string(v)
		case 8:
			colorStr = string(v)
		}
	})
	if content == "" || timeStr == "" {
		return Item{}, false
	}
	sec, err := strconv.ParseFloat(timeStr, 64)
	if err != nil {
		return Item{}, false
	}
	color := defaultColor
	if c := strings.TrimPrefix(strings.TrimSpace(colorStr), "#"); c != "" {
		if len(c) == 6 {
			color = hexColor(c, defaultColor)
		} else if v, err := strconv.ParseInt(c, 10, 64); err == nil {
			color = int(v & 0xFFFFFF)
		}
	}
	return scrollItem(int64(sec*1000), content, color), true
}

// walkProto 遍历一层 protobuf，只把长度型字段交给 fn；其它类型跳过，格式坏了就停。
func walkProto(b []byte, fn func(field int, value []byte)) {
	for len(b) > 0 {
		tag, n := readVarint(b)
		if n <= 0 || tag == 0 {
			return
		}
		b = b[n:]
		field, wire := int(tag>>3), int(tag&7)
		switch wire {
		case wireLen:
			l, n := readVarint(b)
			if n <= 0 || l > uint64(len(b)-n) {
				return
			}
			fn(field, b[n:n+int(l)])
			b = b[n+int(l):]
		case wireVarint:
			_, n := readVarint(b)
			if n <= 0 {
				return
			}
			b = b[n:]
		case wire64:
			if len(b) < 8 {
				return
			}
			b = b[8:]
		case wire32:
			if len(b) < 4 {
				return
			}
			b = b[4:]
		default:
			return
		}
	}
}

// readVarint 返回值和占用字节数；格式错误时字节数为 0。
func readVarint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7F) << (7 * i)
		if b[i]&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}
