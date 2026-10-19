# 爬虫弹窗控件

宿主按文档里的尺寸乘上窗口缩放，放不下时整张缩小。颜色、边距、圆角、下划线没写就不画。字号和颜色都没写时，用当前主题，避免字看不见。

长度单位是文档像素，再乘缩放。`≤0` 的宽、高、字号表示不写。

## 窗口

| 写法 | 作用 |
| --- | --- |
| `title(文字)` | 标题。空则不画标题 |
| `titleSize(n)` | 标题字号。没写则用窗口 `fontSize`，再没有则 14 |
| `size(宽, 高)` | 窗口宽高。高 `≤0` 时按内容增高 |
| `fontSize(n)` | 没单独写字号的控件用这个 |
| `background(色)` / `color(色)` / `border(色)` | 窗口底色、默认字色、描边。底色没写用主题对话框底色 |
| `padding(n)` | 窗口内边距。`0` 也会送出 |
| `radius(n)` | 窗口圆角。没写或 `≤0` 不画圆角 |
| `style(键, 值)` | 写到窗口上，例如 `fontWeight` |
| `timeoutMs(毫秒)` | 超时后关闭 |
| `html(片段)` | 窗口顶部的 HTML |
| `overlay(true)` | 不换掉当前窗，叠在上面 |
| `at(x, y)` / `z(n)` | 窗口相对屏幕的位置和叠放次序。没写坐标则居中 |
| `modal(true/false)` | 是否挡住后面并接受点外面关闭。没写时：没坐标则挡住 |
| `actionAlign(对齐)` | 底栏动作的横向对齐 |
| `actionStyle(字号, 宽, 高)` | 底栏动作没单独写时的字号、宽、高 |

没写窗口宽度时，宿主只用一个缩放盒子：桌面 560，其它 420，并限制在屏幕内。这不是卡片样式。

## 每个控件都能写的键

构造方法上的宽、高、字号，以及 `style(键, 值)`：

| 键 | 作用 |
| --- | --- |
| `width` / `height` | 这个控件的宽高。不写则按内容 |
| `fontSize` | 字号。不写则用窗口字号，再没有则 14 |
| `color` | 字色。勾选、单选、开关、滑动条、进度圈上同时是控件强调色 |
| `background` | 底色 |
| `border` | 描边颜色 |
| `padding` / `margin` | 内边距、外边距。不写则没有 |
| `radius` | 圆角 |
| `align` | `start`、`center`、`end`、`right`（同 `end`）、`space-between`、`between`、`space-around`、`space-evenly`。不写则靠起始一侧 |
| `fontWeight` | `500`–`800` 对应中等、半粗、粗、更粗。低于 500 为常规 |
| `spacing` | 行、列、色点与标题、取色块与色带之间的间距 |
| `weight` | 行里按比例分剩余宽度。列只有写了高度时才按比例分高度 |
| `x` / `y` / `z` | 相对窗口内容区叠放，不再往下排。`at(x, y)`、`z(n)` 是同样的键 |
| `url` | 点按时交给系统打开。`http`、应用 scheme，或 `package:包名` |
| `hint` / `hintColor` | 按钮副标题，或输入框占位字色、取色标签色 |
| `mark` | 按钮左侧色点，颜色值。直径等于该按钮字号 |

`style` 写到刚加上的那个控件。窗口级样式用 `Document.style`。

## 文字 `text`

`text(文字)` 或 `text(文字, 字号)`。多出来的行用 `align` 决定左、中、右。

## 图片 `image`

`image(地址, 宽, 高)`。地址可以是网络图或 `data:` 图。不写宽高则按图本身。

## 输入框 `input`

多行输入就是 `input`，第三个参数为 `true`。没有另外一种控件。

```java
doc.input("note", "多行内容", true, "已有的字", 360, 120, 14)
        .style("background", "#f4f4f5")
        .style("color", "#1c1c1c")
        .style("hintColor", "#8e8e8c")
        .style("radius", 10)
        .style("padding", 12);
```

| 参数或键 | 作用 |
| --- | --- |
| `id` | 提交时的字段名 |
| `placeholder` | 没输入时的提示 |
| `multiline` | `true` 为多行，随内容增高。`false` 为单行 |
| `value` | 初始文字 |
| `width` | 长度。不写则只包住内容，浅色底会缩成一小条 |
| `height` | 可见高度。多行不写高度时跟着文字变高 |
| `fontSize` | 字号 |
| `color` | 输入文字和光标的颜色。光标没写时用主题主色 |
| `hintColor` | 占位字颜色。不写用主题次要色 |
| `background` / `border` / `radius` / `padding` | 框的底、边、圆角、内边距。都不写就看不到框 |
| `password` | 仅 `password(...)` 会置为真，内容默认隐藏 |

`password(id, 占位)` 和 `password(id, 占位, 宽, 高, 字号)` 是单行密码框，不能多行。旁边放一个 `button`，再 `style("toggle", 输入框id)`，点击只在本地切换是否隐藏，不会提交这个按钮。

没有字符个数上限，也没有单独的「列数」。长度用 `width`。要并排多列，用 `row` 把几个输入框放在一起。

## 勾选 `checkbox`、开关 `toggle`、单选 `radio`

- `checkbox(id, 文字, 是否选中)`，可再加字号。
- `toggle(id, 文字, 是否选中)` 画出开关。文字在左，开关在右。`color` 是轨道色，圆钮是控件本身的白色。
- `radio(id, 当前id, 选项...)`。选项是 `id` 加 `label`。

勾选和单选的 `color` 同时用于选中色和文字色。

## 下拉 `select`

`select(id, 当前id, 选项...)`，可再加字号。`background` 是展开列表的底色，不写用主题对话框底色。`color` 是文字色，写了也用作聚焦色。

## 按钮 `button`

`button(id, 文字)` 或 `button(id, 文字, 点后是否关闭)`，可再加宽、高、字号。

没写 `hint` 和 `mark` 时就是一行可点的字，颜色用 `color`，不写则用主题字色。写了 `hint` 或 `mark` 时，左侧是色点（若有），右侧是标题加副标题，这一行撑满所在列。色点和文字的间距用 `spacing`。

`url(地址)` 写在按钮上时，点击先打开地址，再回报按钮 id。

## 链接 `link`

`link(文字, 地址)`。`style` 只有 `underline` 会在字下画线，其它值不画。字色用 `color`。点击交给系统打开。

## 进度 `progress`、滑动条 `slider`

- `progress()`：转圈。`color` 是圈的颜色，不写用主题主色。`align` 决定靠哪一侧。
- `slider(id, 当前值, 最小, 最大)`。`style("step", 步长)` 可写步进。`color` 是已滑过的颜色。提交的是当前数值字符串。

## 取色 `color`

`color(id, "#RRGGBB")`。控件固定为色点加三条色带，标签是「色相」「饱和」「明度」。

| 键 | 作用 |
| --- | --- |
| `height` | 色点直径。不写则等于字号 |
| `fontSize` | 标签字号 |
| `hintColor` | 标签颜色。不写用主题次要色 |
| `spacing` | 色点、标签、色带之间的间距。不写则贴在一起 |
| `border` | 色点描边。不写则没有 |

提交值为 `#RRGGBB`。

## 分隔 `separator`、空白 `spacer`

- `separator()`：一条横线。`color` 是线色，不写用主题轮廓色。`height` 是线粗，不写则为 1 个缩放像素。
- `spacer(高度)`：垂直空白。高度 `≤0` 不画。

## 行、列、分组

- `row(块)` 或 `row(间距, 块)`：横向排列。`align` 是主轴对齐。
- `column(块)` 或 `column(间距, 块)`：纵向排列。
- `group(标题, 块)` 或 `group(标题, 间距, 块)`：带标题的列。标题用该组的 `color`、`fontSize`、`fontWeight`。

子项不写 `width` 时按内容收缩。要撑满一行，写 `width`，或在 `row` 里 `weight(n)`。

## HTML `html`

`html(片段)` 或窗口的 `html`。不执行脚本，不嵌网页。`href="action:某id"` 回报该动作，其它链接交给系统打开。`position:absolute` 或 `fixed` 时按 `left`、`top`、`z-index` 叠在正文上。字色和字号用元素上的 `color`、`fontSize`。

## 底栏动作 `action`

`action(id, 文字, 点后是否关闭)`，可再加宽、高、字号，或 `action(id, 文字, 是否关闭, 样式名, 颜色)`。颜色会画上。样式名目前不改变外观；底色、圆角、内边距要写在元素按钮上，不要指望这个样式名。

对齐用窗口的 `actionAlign`。不写则从起始一侧排。

## 提交回去的值

点按钮或底栏动作时，宿主把当前值一并送回：

| 控件 | 内容 |
| --- | --- |
| 输入框、滑动条、取色 | `id` → 文本 |
| 勾选、开关 | `id` → `true` 或 `false` |
| 单选、下拉 | `id` → 选中的选项 id |

密码是否隐藏只存在本地，键名以 `obscure:` 开头，不会送回。
