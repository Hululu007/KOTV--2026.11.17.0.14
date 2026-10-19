package com.github.catvod.utils;

import com.github.catvod.crawler.SpiderDebug;

import java.util.Collections;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;
import java.util.function.Consumer;

/**
 * Declarative UI protocol for spider scripts. Host only renders frozen primitives and
 * reports shown/closed — it never interprets content (image vs anything else) or action ids.
 * An html field or html element is drawn as markup. Scripts and iframes in that markup
 * are not run. A link whose href is {@code action:id} is reported as that action id.
 * {@code overlay} keeps the current window and draws another one above it.
 * Element {@code x}/{@code y} and HTML {@code position:absolute} paint boxes on top of each other.
 *
 * <p>Frozen host element types: text, image, input, checkbox, radio, select, button,
 * link, progress, slider, color, separator, spacer, space, row, column, group, html, switch.
 * A button or row {@code url} is opened by the system (http, app scheme, or {@code package:name}).
 * Bridge: {@link #show}/{@link #close} handshake only; scripts own documents and replies.
 */
public final class UiBridge {

    public interface Handle {
        void dispose();
    }

    /** Host handshake failed (shown/closed timeout or post failed). */
    public static final class ShowFailedException extends RuntimeException {
        public ShowFailedException(String message) {
            super(message);
        }
    }

    /** One option for radio / select. */
    public static final class Option {
        public final String id;
        public final String label;

        public Option(String id, String label) {
            this.id = id == null ? "" : id;
            this.label = label == null || label.trim().isEmpty() ? this.id : label;
        }

        public static Option of(String id) {
            return new Option(id, id);
        }

        public static Option of(String id, String label) {
            return new Option(id, label);
        }
    }

    /**
     * Mutable element list for a document or nested layout block.
     */
    public static final class Elements {
        private final List<Map<String, Object>> target;

        Elements(List<Map<String, Object>> target) {
            this.target = target;
        }

        public Elements text(String text) {
            addText(target, text, 0);
            return this;
        }

        public Elements text(String text, int fontSize) {
            addText(target, text, fontSize);
            return this;
        }

        public Elements image(String source, int width, int height) {
            addImage(target, source, width, height);
            return this;
        }

        public Elements input(String id, String placeholder, boolean multiline) {
            addInput(target, id, placeholder, multiline, false, "", 0, 0, 0);
            return this;
        }

        public Elements input(String id, String placeholder, boolean multiline, String value) {
            addInput(target, id, placeholder, multiline, false, value, 0, 0, 0);
            return this;
        }

        /** width/height/fontSize：≤0 表示该项不指定。不补外观。窗口没写宽度时，宿主只用缩放盒子。 */
        public Elements input(String id, String placeholder, boolean multiline,
                              int width, int height, int fontSize) {
            addInput(target, id, placeholder, multiline, false, "", width, height, fontSize);
            return this;
        }

        public Elements input(String id, String placeholder, boolean multiline, String value,
                              int width, int height, int fontSize) {
            addInput(target, id, placeholder, multiline, false, value, width, height, fontSize);
            return this;
        }

        public Elements password(String id, String placeholder) {
            addInput(target, id, placeholder, false, true, "", 0, 0, 0);
            return this;
        }

        public Elements password(String id, String placeholder, int width, int height, int fontSize) {
            addInput(target, id, placeholder, false, true, "", width, height, fontSize);
            return this;
        }

        public Elements checkbox(String id, String text, boolean checked) {
            return checkbox(id, text, checked, 0);
        }

        public Elements checkbox(String id, String text, boolean checked, int fontSize) {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "checkbox");
            element.put("id", id);
            element.put("text", text == null ? "" : text);
            element.put("checked", checked);
            putStyle(element, 0, 0, fontSize);
            target.add(element);
            return this;
        }

        public Elements toggle(String id, String text, boolean checked) {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "switch");
            element.put("id", id);
            element.put("text", text == null ? "" : text);
            element.put("checked", checked);
            target.add(element);
            return this;
        }

        public Elements radio(String id, String selected, Option... options) {
            return radio(id, selected, options == null ? Collections.emptyList() : Arrays.asList(options));
        }

        public Elements radio(String id, String selected, List<Option> options) {
            return radio(id, selected, options, 0);
        }

        public Elements radio(String id, String selected, List<Option> options, int fontSize) {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "radio");
            element.put("id", id);
            if (selected != null) element.put("value", selected);
            element.put("options", toOptionMaps(options));
            putStyle(element, 0, 0, fontSize);
            target.add(element);
            return this;
        }

        public Elements select(String id, String selected, Option... options) {
            return select(id, selected, options == null ? Collections.emptyList() : Arrays.asList(options));
        }

        public Elements select(String id, String selected, List<Option> options) {
            return select(id, selected, options, 0, 0, 0);
        }

        public Elements select(String id, String selected, List<Option> options,
                               int width, int height, int fontSize) {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "select");
            element.put("id", id);
            if (selected != null) element.put("value", selected);
            element.put("options", toOptionMaps(options));
            putStyle(element, width, height, fontSize);
            target.add(element);
            return this;
        }

        public Elements button(String id, String label) {
            return button(id, label, false, 0, 0, 0);
        }

        public Elements button(String id, String label, boolean dismiss) {
            return button(id, label, dismiss, 0, 0, 0);
        }

        public Elements button(String id, String label, boolean dismiss,
                               int width, int height, int fontSize) {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "button");
            element.put("id", id);
            element.put("text", label == null ? "" : label);
            element.put("dismiss", dismiss);
            putStyle(element, width, height, fontSize);
            target.add(element);
            return this;
        }

        /** 下划线只在 style 为 underline 时画。 */
        public Elements link(String text, String url) {
            return link(text, url, "", 0, 0, 0);
        }

        public Elements link(String text, String url, String style) {
            return link(text, url, style, 0, 0, 0);
        }

        public Elements link(String text, String url, String style,
                             int width, int height, int fontSize) {
            if (url == null || url.trim().isEmpty()) return this;
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "link");
            element.put("text", text == null || text.trim().isEmpty() ? url : text);
            element.put("url", url.trim());
            if (style != null && !style.trim().isEmpty()) element.put("style", style.trim());
            putStyle(element, width, height, fontSize);
            target.add(element);
            return this;
        }

        public Elements html(String html) {
            if (html == null || html.trim().isEmpty()) return this;
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "html");
            element.put("html", html);
            target.add(element);
            return this;
        }

        public Elements weight(int weight) {
            if (target.isEmpty() || weight <= 0) return this;
            target.get(target.size() - 1).put("weight", weight);
            return this;
        }

        /** 上一个按钮或行被点按时，交给系统打开。http、应用 scheme，或 {@code package:包名}。 */
        public Elements url(String url) {
            if (target.isEmpty() || url == null || url.trim().isEmpty()) return this;
            target.get(target.size() - 1).put("url", url.trim());
            return this;
        }

        /** 写到上一个元素上。键如 color、background、padding、margin、radius、align、border。 */
        public Elements style(String key, Object value) {
            if (target.isEmpty() || key == null || key.isEmpty() || value == null) return this;
            target.get(target.size() - 1).put(key, value);
            return this;
        }

        /** 上一个元素按坐标叠放，不再跟着文档往下排。 */
        public Elements at(int x, int y) {
            if (target.isEmpty()) return this;
            Map<String, Object> last = target.get(target.size() - 1);
            last.put("x", x);
            last.put("y", y);
            return this;
        }

        public Elements z(int z) {
            if (target.isEmpty()) return this;
            target.get(target.size() - 1).put("z", z);
            return this;
        }

        public Elements separator() {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "separator");
            target.add(element);
            return this;
        }

        public Elements spacer(int height) {
            if (height <= 0) return this;
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "spacer");
            element.put("height", height);
            target.add(element);
            return this;
        }

        /** Indeterminate progress indicator; host draws chrome only, no caption. */
        public Elements progress() {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "progress");
            target.add(element);
            return this;
        }

        public Elements slider(String id, int value, int min, int max) {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "slider");
            element.put("id", id == null ? "" : id);
            element.put("value", value);
            element.put("min", min);
            element.put("max", max);
            target.add(element);
            return this;
        }

        public Elements color(String id, String value) {
            Map<String, Object> element = new LinkedHashMap<>();
            element.put("type", "color");
            element.put("id", id == null ? "" : id);
            element.put("value", value == null ? "" : value);
            target.add(element);
            return this;
        }

        public Elements row(Consumer<Elements> block) {
            return row(0, block);
        }

        public Elements row(int spacing, Consumer<Elements> block) {
            addLayout(target, "row", null, spacing, block);
            return this;
        }

        public Elements column(Consumer<Elements> block) {
            return column(0, block);
        }

        public Elements column(int spacing, Consumer<Elements> block) {
            addLayout(target, "column", null, spacing, block);
            return this;
        }

        public Elements group(String title, Consumer<Elements> block) {
            return group(title, 0, block);
        }

        public Elements group(String title, int spacing, Consumer<Elements> block) {
            addLayout(target, "group", title, spacing, block);
            return this;
        }
    }

    /**
     * Declarative window builder. Compose with add* then {@link #show(String, Document)}.
     */
    public static final class Document {
        public String title = "";
        /** 窗口固定宽高（逻辑像素）；脚本必填意图尺寸，宿主只做屏幕上限裁剪。 */
        public int width = 0;
        public int height = 0;
        /** 文档级默认字号；元素未单独指定 fontSize 时宿主用它。 */
        public int fontSize = 0;
        public int titleSize = 0;
        /** 底栏 action 默认样式。 */
        public int actionFontSize = 0;
        public int actionWidth = 0;
        public int actionHeight = 0;
        public long timeoutMs = 120_000;
        /** 自由排版。宿主只画标记，不执行里面的脚本。 */
        public String html = "";
        public String background = "";
        public String color = "";
        public String border = "";
        public int padding = 0;
        private boolean paddingSet;
        public int radius = -1;
        /** 为 true 时叠在同 kind 已有窗上面，不关掉它。 */
        public boolean overlay;
        public Integer x;
        public Integer y;
        public int z;
        /** null 表示：没给坐标时点外面关掉这层，给了坐标则不挡下面的窗。 */
        public Boolean modal;
        public String actionAlign = "";
        public final Map<String, Object> style = new LinkedHashMap<>();
        public final List<Map<String, Object>> elements = new ArrayList<>();
        public final List<Map<String, Object>> actions = new ArrayList<>();
        private final Elements els = new Elements(elements);

        public Document title(String title) {
            if (title != null && !title.trim().isEmpty()) this.title = title;
            return this;
        }

        public Document size(int width, int height) {
            if (width > 0) this.width = width;
            if (height > 0) this.height = height;
            return this;
        }

        public Document fontSize(int fontSize) {
            if (fontSize > 0) this.fontSize = fontSize;
            return this;
        }

        public Document titleSize(int titleSize) {
            if (titleSize > 0) this.titleSize = titleSize;
            return this;
        }

        public Document actionStyle(int fontSize, int width, int height) {
            if (fontSize > 0) this.actionFontSize = fontSize;
            if (width > 0) this.actionWidth = width;
            if (height > 0) this.actionHeight = height;
            return this;
        }

        public Document timeoutMs(long timeoutMs) {
            this.timeoutMs = timeoutMs;
            return this;
        }

        public Document html(String html) {
            this.html = html == null ? "" : html;
            return this;
        }

        /** 在控件列表里插一块标记，和窗口级 html 可以同时有。 */
        public Document markup(String html) {
            els.html(html);
            return this;
        }

        public Document background(String color) {
            this.background = color == null ? "" : color.trim();
            return this;
        }

        public Document color(String color) {
            this.color = color == null ? "" : color.trim();
            return this;
        }

        public Document border(String color) {
            this.border = color == null ? "" : color.trim();
            return this;
        }

        public Document padding(int padding) {
            if (padding >= 0) {
                this.padding = padding;
                this.paddingSet = true;
            }
            return this;
        }

        public Document radius(int radius) {
            if (radius >= 0) this.radius = radius;
            return this;
        }

        public Document overlay(boolean overlay) {
            this.overlay = overlay;
            return this;
        }

        public Document at(int x, int y) {
            this.x = x;
            this.y = y;
            return this;
        }

        public Document z(int z) {
            this.z = z;
            return this;
        }

        public Document modal(boolean modal) {
            this.modal = modal;
            return this;
        }

        /** 窗口级样式。键如 background、color、border、padding、radius、margin、align。 */
        public Document style(String key, Object value) {
            if (key == null || key.isEmpty() || value == null) return this;
            if ("id".equals(key) || "elements".equals(key) || "actions".equals(key)) return this;
            style.put(key, value);
            return this;
        }

        public Document text(String text) {
            els.text(text);
            return this;
        }

        public Document text(String text, int fontSize) {
            els.text(text, fontSize);
            return this;
        }

        public Document image(String source, int width, int height) {
            els.image(source, width, height);
            return this;
        }

        public Document input(String id, String placeholder, boolean multiline) {
            els.input(id, placeholder, multiline);
            return this;
        }

        public Document input(String id, String placeholder, boolean multiline, String value) {
            els.input(id, placeholder, multiline, value);
            return this;
        }

        public Document input(String id, String placeholder, boolean multiline,
                              int width, int height, int fontSize) {
            els.input(id, placeholder, multiline, width, height, fontSize);
            return this;
        }

        public Document input(String id, String placeholder, boolean multiline, String value,
                              int width, int height, int fontSize) {
            els.input(id, placeholder, multiline, value, width, height, fontSize);
            return this;
        }

        public Document password(String id, String placeholder) {
            els.password(id, placeholder);
            return this;
        }

        public Document password(String id, String placeholder, int width, int height, int fontSize) {
            els.password(id, placeholder, width, height, fontSize);
            return this;
        }

        public Document checkbox(String id, String text, boolean checked) {
            els.checkbox(id, text, checked);
            return this;
        }

        public Document checkbox(String id, String text, boolean checked, int fontSize) {
            els.checkbox(id, text, checked, fontSize);
            return this;
        }

        public Document toggle(String id, String text, boolean checked) {
            els.toggle(id, text, checked);
            return this;
        }

        public Document radio(String id, String selected, Option... options) {
            els.radio(id, selected, options);
            return this;
        }

        public Document radio(String id, String selected, List<Option> options) {
            els.radio(id, selected, options);
            return this;
        }

        public Document radio(String id, String selected, List<Option> options, int fontSize) {
            els.radio(id, selected, options, fontSize);
            return this;
        }

        public Document select(String id, String selected, Option... options) {
            els.select(id, selected, options);
            return this;
        }

        public Document select(String id, String selected, List<Option> options) {
            els.select(id, selected, options);
            return this;
        }

        public Document select(String id, String selected, List<Option> options,
                               int width, int height, int fontSize) {
            els.select(id, selected, options, width, height, fontSize);
            return this;
        }

        public Document button(String id, String label) {
            els.button(id, label);
            return this;
        }

        public Document button(String id, String label, boolean dismiss) {
            els.button(id, label, dismiss);
            return this;
        }

        public Document button(String id, String label, boolean dismiss,
                               int width, int height, int fontSize) {
            els.button(id, label, dismiss, width, height, fontSize);
            return this;
        }

        public Document link(String text, String url) {
            els.link(text, url);
            return this;
        }

        public Document link(String text, String url, String style) {
            els.link(text, url, style);
            return this;
        }

        public Document link(String text, String url, String style,
                             int width, int height, int fontSize) {
            els.link(text, url, style, width, height, fontSize);
            return this;
        }

        public Document separator() {
            els.separator();
            return this;
        }

        public Document spacer(int height) {
            els.spacer(height);
            return this;
        }

        public Document progress() {
            els.progress();
            return this;
        }

        public Document slider(String id, int value, int min, int max) {
            els.slider(id, value, min, max);
            return this;
        }

        public Document color(String id, String value) {
            els.color(id, value);
            return this;
        }

        public Document row(Consumer<Elements> block) {
            els.row(block);
            return this;
        }

        public Document row(int spacing, Consumer<Elements> block) {
            els.row(spacing, block);
            return this;
        }

        public Document column(Consumer<Elements> block) {
            els.column(block);
            return this;
        }

        public Document column(int spacing, Consumer<Elements> block) {
            els.column(spacing, block);
            return this;
        }

        public Document group(String title, Consumer<Elements> block) {
            els.group(title, block);
            return this;
        }

        public Document group(String title, int spacing, Consumer<Elements> block) {
            els.group(title, spacing, block);
            return this;
        }

        public Document action(String id, String label, boolean dismiss) {
            actions.add(UiBridge.action(id, label, dismiss, 0, 0, 0));
            return this;
        }

        public Document action(String id, String label, boolean dismiss,
                               int width, int height, int fontSize) {
            actions.add(UiBridge.action(id, label, dismiss, width, height, fontSize));
            return this;
        }

        /** look、color 写到这条动作上。 */
        public Document action(String id, String label, boolean dismiss, String look, String color) {
            Map<String, Object> item = UiBridge.action(id, label, dismiss, 0, 0, 0);
            if (look != null && !look.trim().isEmpty()) item.put("style", look.trim());
            if (color != null && !color.trim().isEmpty()) item.put("color", color.trim());
            actions.add(item);
            return this;
        }

        /** start、end、center、space-between、space-around、space-evenly。没写时从起始一侧排。right 与 end 相同。 */
        public Document actionAlign(String align) {
            this.actionAlign = align == null ? "" : align.trim();
            return this;
        }

        public Document url(String url) {
            els.url(url);
            return this;
        }

        public Document weight(int weight) {
            els.weight(weight);
            return this;
        }
    }

    /** 同一 kind 从底到顶的 session。overlay 往上叠，普通 show 先清掉这一叠。 */
    private static final ConcurrentHashMap<String, List<String>> sessionsByKind = new ConcurrentHashMap<>();
    private static final AtomicLong nextSession = new AtomicLong();

    private UiBridge() {
    }

    /**
     * 向宿主开窗：脚本提供完整 {@link Document}。
     * 宿主只渲染原语并回报 shown / closed；按钮动作与内容语义由脚本自行
     * {@link Util#takeUiReply(String)} 消费。失败抛 {@link ShowFailedException}。
     */
    public static Handle show(String kind, Document document) {
        String key = kindKey(kind);
        String session;
        try {
            session = openSession(kind, document != null && document.overlay);
        } catch (ShowFailedException e) {
            SpiderDebug.log("UiBridge openSession: " + e.getMessage());
            Util.notify("弹窗关闭确认超时，请重试");
            throw e;
        }
        Document doc = document == null ? new Document() : document;
        try {
            showDocument(session, doc);
        } catch (ShowFailedException e) {
            SpiderDebug.log("UiBridge showDocument: " + e.getMessage());
            forget(key, session);
            closeDocument(session);
            Util.waitUiAction(session, "closed", Util.UI_CLOSE_WAIT_MS);
            Util.notify("弹窗未能显示，请重试");
            throw e;
        }
        return handleFor(key, session);
    }

    /**
     * 脚本主动关窗并等宿主 closed。成功返回 true。
     * toast 等由脚本自行 {@link Util#notify}。
     */
    public static boolean close(String kind) {
        String key = kindKey(kind);
        String session = currentSession(kind);
        if (session.isEmpty()) return true;
        return closeFrom(key, session);
    }


    /**
     * 当前 kind 弹窗对应的客户端平台（android/ios/…）。
     * 以 Flutter 前端为准；按 session 隔离。须在 show/shown 握手成功后读取。
     */
    public static String hostPlatform(String kind) {
        return Util.hostPlatform(currentSession(kind));
    }

    public static boolean hostDesktop(String kind) {
        return Util.hostDesktop(currentSession(kind));
    }

    public static void dispose(Handle handle) {
        if (handle == null) return;
        try {
            handle.dispose();
        } catch (Throwable ignored) {
        }
    }

    /** 脚本用的内部 kind；跨宿主边界只有不透明 session id。按 Scope 隔离。 */
    public static String currentSession(String kind) {
        return top(kindKey(kind));
    }

    private static List<String> stack(String key) {
        return sessionsByKind.computeIfAbsent(key, k -> new ArrayList<>());
    }

    private static String top(String key) {
        List<String> stack = sessionsByKind.get(key);
        if (stack == null) return "";
        synchronized (stack) {
            return stack.isEmpty() ? "" : stack.get(stack.size() - 1);
        }
    }

    private static void forget(String key, String session) {
        List<String> stack = sessionsByKind.get(key);
        if (stack == null) return;
        synchronized (stack) {
            stack.remove(session);
        }
    }

    /** 关掉 session 以及叠在它上面的层，从顶到底。栈空了才作废还在发出的界面消息。 */
    private static boolean closeFrom(String key, String session) {
        List<String> closing = new ArrayList<>();
        List<String> stack = sessionsByKind.get(key);
        if (stack == null) {
            closing.add(session);
        } else {
            synchronized (stack) {
                int idx = stack.indexOf(session);
                if (idx < 0) {
                    closing.add(session);
                } else {
                    closing.addAll(stack.subList(idx, stack.size()));
                    stack.subList(idx, stack.size()).clear();
                }
            }
        }
        boolean ok = true;
        for (int i = closing.size() - 1; i >= 0; i--) {
            String id = closing.get(i);
            closeDocument(id);
            if (!Util.waitUiAction(id, "closed", Util.UI_CLOSE_WAIT_MS)) ok = false;
            Util.clearHostClientInfo(id);
        }
        if (top(key).isEmpty()) Util.clearPendingUiNotify();
        return ok;
    }

    private static String openSession(String kind, boolean overlay) {
        String key = kindKey(kind);
        String session = "ui-" + Long.toUnsignedString(nextSession.incrementAndGet(), 36)
                + "-" + Long.toUnsignedString(System.nanoTime(), 36);
        List<String> stack = stack(key);
        List<String> closing = new ArrayList<>();
        synchronized (stack) {
            if (!overlay) {
                closing.addAll(stack);
                stack.clear();
            }
            stack.add(session);
        }
        for (int i = closing.size() - 1; i >= 0; i--) {
            String previous = closing.get(i);
            closeDocument(previous);
            if (!Util.waitUiAction(previous, "closed", Util.UI_CLOSE_WAIT_MS)) {
                forget(key, session);
                throw new ShowFailedException("wait closed timeout for " + previous);
            }
            Util.clearHostClientInfo(previous);
        }
        return session;
    }

    private static void showDocument(String session, Document doc) {
        Map<String, Object> document = new LinkedHashMap<>();
        document.put("id", session);
        document.put("title", doc.title == null ? "" : doc.title);
        document.put("width", doc.width);
        document.put("height", doc.height);
        if (doc.fontSize > 0) document.put("fontSize", doc.fontSize);
        if (doc.titleSize > 0) document.put("titleSize", doc.titleSize);
        if (doc.actionFontSize > 0) document.put("actionFontSize", doc.actionFontSize);
        if (doc.actionWidth > 0) document.put("actionWidth", doc.actionWidth);
        if (doc.actionHeight > 0) document.put("actionHeight", doc.actionHeight);
        document.put("timeoutMs", doc.timeoutMs);
        if (doc.html != null && !doc.html.isEmpty()) document.put("html", doc.html);
        if (doc.background != null && !doc.background.isEmpty()) document.put("background", doc.background);
        if (doc.color != null && !doc.color.isEmpty()) document.put("color", doc.color);
        if (doc.border != null && !doc.border.isEmpty()) document.put("border", doc.border);
        if (doc.paddingSet) document.put("padding", doc.padding);
        if (doc.radius >= 0) document.put("radius", doc.radius);
        if (doc.overlay) document.put("overlay", true);
        if (doc.x != null) document.put("x", doc.x);
        if (doc.y != null) document.put("y", doc.y);
        if (doc.z != 0) document.put("z", doc.z);
        if (doc.modal != null) document.put("modal", doc.modal);
        if (doc.actionAlign != null && !doc.actionAlign.isEmpty()) document.put("actionAlign", doc.actionAlign);
        for (Map.Entry<String, Object> extra : doc.style.entrySet()) {
            String key = extra.getKey();
            if (key == null || "id".equals(key) || "elements".equals(key) || "actions".equals(key)) continue;
            document.put(key, extra.getValue());
        }
        document.put("elements", doc.elements);
        document.put("actions", doc.actions);
        Util.notifySync("UI:" + Json.toJson(document));
        if (!Util.waitUiAction(session, "shown", Util.UI_HANDSHAKE_MS)) {
            throw new ShowFailedException("wait shown timeout for " + session);
        }
    }

    private static void closeDocument(String session) {
        Map<String, Object> command = new LinkedHashMap<>();
        command.put("id", session);
        Util.notifySync("UI_CLOSE:" + Json.toJson(command));
    }

    private static void putStyle(Map<String, Object> element, int width, int height, int fontSize) {
        if (width > 0) element.put("width", width);
        if (height > 0) element.put("height", height);
        if (fontSize > 0) element.put("fontSize", fontSize);
    }

    private static void addLayout(List<Map<String, Object>> target, String type, String title,
                                  int spacing, Consumer<Elements> block) {
        List<Map<String, Object>> children = new ArrayList<>();
        if (block != null) block.accept(new Elements(children));
        Map<String, Object> element = new LinkedHashMap<>();
        element.put("type", type);
        if (title != null && !title.trim().isEmpty()) element.put("text", title);
        if (spacing > 0) element.put("spacing", spacing);
        element.put("children", children);
        target.add(element);
    }

    private static void addText(List<Map<String, Object>> elements, String text, int fontSize) {
        if (text == null || text.trim().isEmpty()) return;
        Map<String, Object> element = new LinkedHashMap<>();
        element.put("type", "text");
        element.put("text", text);
        putStyle(element, 0, 0, fontSize);
        elements.add(element);
    }

    private static void addImage(List<Map<String, Object>> elements, String source, int width, int height) {
        if (source == null || source.trim().isEmpty()) return;
        Map<String, Object> element = new LinkedHashMap<>();
        element.put("type", "image");
        element.put("source", source);
        putStyle(element, width, height, 0);
        elements.add(element);
    }

    private static void addInput(List<Map<String, Object>> elements, String id, String placeholder,
                                 boolean multiline, boolean password, String value,
                                 int width, int height, int fontSize) {
        Map<String, Object> element = new LinkedHashMap<>();
        element.put("type", "input");
        element.put("id", id);
        element.put("placeholder", placeholder == null ? "" : placeholder);
        element.put("multiline", multiline);
        element.put("password", password);
        if (value != null && !value.isEmpty()) element.put("value", value);
        putStyle(element, width, height, fontSize);
        elements.add(element);
    }

    private static List<Map<String, Object>> toOptionMaps(List<Option> options) {
        List<Map<String, Object>> list = new ArrayList<>();
        if (options == null) return list;
        for (Option option : options) {
            if (option == null) continue;
            Map<String, Object> item = new LinkedHashMap<>();
            item.put("id", option.id);
            item.put("label", option.label);
            list.add(item);
        }
        return list;
    }

    private static Map<String, Object> action(String id, String label, boolean dismiss,
                                              int width, int height, int fontSize) {
        Map<String, Object> action = new LinkedHashMap<>();
        action.put("id", id);
        action.put("label", label);
        action.put("dismiss", dismiss);
        putStyle(action, width, height, fontSize);
        return action;
    }

    private static Handle handleFor(String kind, String session) {
        return () -> closeFrom(kind, session);
    }

    private static String normalizeKind(String kind) {
        if (kind == null || kind.trim().isEmpty()) return "ui";
        return kind.trim().toLowerCase();
    }

    /** kind 按 Scope（优先 userId）隔离；已带分隔符的 key 不再二次前缀。 */
    private static String kindKey(String kind) {
        String k = normalizeKind(kind);
        if (k.indexOf('\u0001') >= 0) return k;
        String scope = Util.scopeId();
        if (scope == null || scope.isEmpty()) return k;
        return scope + '\u0001' + k;
    }

}
