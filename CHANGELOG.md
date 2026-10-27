# Changelog

## v0.2.2

- 下载页边距与首页一致；播放页查看任务叠层打开，返回仍回播放。
- 主 Tab：桌面还原 Material 转场，安卓关闭切换动画；下载设置与文案收拢。
- 「我的」检测爬虫真正调用引擎检测；已入队剧集不可再选（引擎侧去重）。
- 点播首页/分类、目录、历史、收藏、下载支持下拉刷新；空列表也可下拉。
- 目录浏览只走空 path 列表；版本/tools 文案与下载选项标签再收拢。

## v0.2.1

- iOS 没有本机引擎时不再把 127.0.0.1 探满超时。远端连不上就停，不清掉已保存的地址。连不上或登录失败弹窗说明，登录框保持打开。已有地址填进连接框；地址可用时先登录，取消后才回到连接页。
- Win7 系统全屏回到消息循环再改子窗口，退出时按进入前的窗口矩形还原。

## v0.2.0

- 版本号收到 0.2.0。健康检查和「我的」页跟引擎版本走，不再各写一份。
- iOS 顶栏留在状态栏下面，源名和海报里的彩色表情能显示。发行文件名跟这份版本号。

## v0.1.0

- 首个 GitHub Actions 多平台发行包（Windows / Linux / macOS arm64 / amd64）
- Windows 捆绑运行时（Win7 尽力兼容）：Temurin JRE（含 Win10 UCRT + api-ms-win-core-path）、PythonVista、Chromium-for-windows-7-REWORK、FFmpeg 7.0、MSVCRT MinGW（QuickJS CGO）；Windows 主程序改用 go-legacy-win7 构建
- 捆绑 JRE / Python / Chromium / FFmpeg；QuickJS 内嵌主程序
- JAR 爬虫改为捆绑 JRE 常驻 `spider-bridge --serve`：JVM 只启动一次、与 UI 进程隔离，崩溃可自动拉起
- 详情/直播页内嵌播放：默认 FVP（桌面）/ ExoPlayer（Android）；内置 MPV 改原生通道
- Windows QuickJS(CGO)：MSVCRT MinGW、`_WIN32_WINNT=0x0601`、static-libgcc，并做 Win7 依赖门禁
