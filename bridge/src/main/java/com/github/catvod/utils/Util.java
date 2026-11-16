package com.github.catvod.utils;

import com.github.catvod.Init;
import com.github.catvod.Proxy;
import com.github.catvod.crawler.SpiderDebug;
import com.github.catvod.net.OkHttp;
import com.github.catvod.net.OkResult;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;
import org.apache.commons.lang3.StringUtils;
import org.json.JSONObject;

import java.io.IOException;
import java.math.BigInteger;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.text.SimpleDateFormat;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.Collection;
import java.util.Date;
import java.util.HashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public class Util {
    /**
     * 当前 JAR 调用所属的 Flutter clientId / Scope（由 SpiderBridge 从请求注入）。
     * InheritableThreadLocal：jar 里 new Thread 的子线程继承 scope。
     * 线程池须靠 bindCallScope；NetProfiles 无绑定时只回落默认桶，禁止跨租户兜底。
     */
    private static final ThreadLocal<String> CLIENT_ID_TL = new InheritableThreadLocal<>();
    /** 远端鉴权用户 id（稳定 UUID）；界面消息 / Scope 路由用，不进 cookie 目录名。 */
    private static final ThreadLocal<String> USER_ID_TL = new InheritableThreadLocal<>();
    /** 远端登录名；仅 {@link #tvScopeId()}/{@link Path#tv} 目录用，避免随账号 id 重建而变。 */
    private static final ThreadLocal<String> USERNAME_TL = new InheritableThreadLocal<>();

    private static final AtomicLong UI_NOTIFY_EPOCH = new AtomicLong();
    private static final ExecutorService UI_MESSAGES = Executors.newSingleThreadExecutor(r -> {
        Thread thread = new Thread(r, "catvod-ui-messages");
        thread.setDaemon(true);
        return thread;
    });
    public static final String patternAli = "(https:\\/\\/www\\.aliyundrive\\.com\\/s\\/[^\"]+|https:\\/\\/www\\.alipan\\.com\\/s\\/[^\"]+)";
    public static final String patternQuark = "(https:\\/\\/pan\\.quark\\.cn\\/s\\/[^\"]+)";
    public static final String patternUC = "(https:\\/\\/drive\\.uc\\.cn\\/s\\/[^\"]+)";
    public static final Pattern RULE = Pattern.compile("http((?!http).){12,}?\\.(m3u8|mp4|flv|avi|mkv|rm|wmv|mpg|m4a|mp3)\\?.*|http((?!http).){12,}\\.(m3u8|mp4|flv|avi|mkv|rm|wmv|mpg|m4a|mp3)|http((?!http).)*?video/tos*");
    public static final String CHROME = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36";
    public static final String SAFARI = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.33";
    public static final String ACCEPT = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7";
    public static final List<String> MEDIA = Arrays.asList(
            "mp4", "mkv", "mov", "m4v", "webm", "wmv", "flv", "avi", "iso", "mpg", "mpeg",
            "ts", "m2ts", "mp3", "aac", "flac", "m4a", "ape", "ogg", "wav", "wma",
            "rm", "rmvb", "asf", "dts", "dsf", "dff", "m3u8", "mpd"
    );
    public static final List<String> SUB = Arrays.asList("srt", "ass", "ssa", "vtt");

    private static HashMap<String, String> webHttpHeaderMap;

    public static final String CLIENT_ID = "76917ccccd4441c39457a04f6084fb2f";

    public static boolean isVip(String url) {
        List<String> hosts = Arrays.asList("iqiyi.com", "v.qq.com", "youku.com", "le.com", "tudou.com", "mgtv.com", "sohu.com", "acfun.cn", "bilibili.com", "baofeng.com", "pptv.com");
        for (String host : hosts) if (url.contains(host)) return true;
        return false;
    }

    public static boolean isBlackVodUrl(String url) {
        List<String> hosts = Arrays.asList("973973.xyz", ".fit:");
        for (String host : hosts) if (url.contains(host)) return true;
        return false;
    }

    public static boolean isVideoFormat(String url) {
        if (url.contains("url=http") || url.contains(".js") || url.contains(".css") || url.contains(".html"))
            return false;
        return RULE.matcher(url).find();
    }

    public static String findByRegex(String regex, String content, Integer groupCount) {
        // 创建 Pattern 对象
        Pattern r = Pattern.compile(regex);

        // 现在创建 matcher 对象
        Matcher m = r.matcher(content);
        if (m.find()) {
            return m.group(groupCount);
        } else {
            return "";
        }
    }

    public static byte[] toUtf8(byte[] bytes) {
        return new String(bytes, StandardCharsets.UTF_8).getBytes();
    }

    public static boolean isSub(String ext) {
        return ext != null && SUB.contains(ext.toLowerCase(Locale.ROOT));
    }

    public static boolean isMedia(String text) {
        return MEDIA.contains(getExt(text).toLowerCase(Locale.ROOT));
    }

    public static String getExt(String name) {
        if (name == null || !name.contains(".")) return name == null ? "" : name;
        return name.substring(name.lastIndexOf(".") + 1);
    }

    public static String getSize(double size) {
        if (size <= 0) return "";
        if (size > 1024 * 1024 * 1024 * 1024.0) {
            size /= (1024 * 1024 * 1024 * 1024.0);
            return String.format(Locale.getDefault(), "%.2f%s", size, "TB");
        } else if (size > 1024 * 1024 * 1024.0) {
            size /= (1024 * 1024 * 1024.0);
            return String.format(Locale.getDefault(), "%.2f%s", size, "GB");
        } else if (size > 1024 * 1024.0) {
            size /= (1024 * 1024.0);
            return String.format(Locale.getDefault(), "%.2f%s", size, "MB");
        } else {
            size /= 1024.0;
            return String.format(Locale.getDefault(), "%.2f%s", size, "KB");
        }
    }

    //todo
    public static String fixUrl(String base, String src) {
        if (src.startsWith("//")) {
            URI parse = URI.create(base);
            return parse.getScheme() + ":" + src;
        } else if (!src.contains("://")) {
            URI parse = URI.create(base);
            return parse.getScheme() + "://" + parse.getHost() + src;
        } else {
            return src;
        }
    }

    public static String removeExt(String text) {
        return text.contains(".") ? text.substring(0, text.lastIndexOf(".")) : text;
    }

    public static String substring(String text) {
        return substring(text, 1);
    }

    public static String substring(String text, int num) {
        if (text != null && text.length() > num) {
            return text.substring(0, text.length() - num);
        } else {
            return text;
        }
    }

    public static String getVar(String data, String param) {
        for (String var : data.split("var")) if (var.contains(param)) return checkVar(var);
        return "";
    }

    private static String checkVar(String var) {
        if (var.contains("'")) return var.split("'")[1];
        if (var.contains("\"")) return var.split("\"")[1];
        return "";
    }

    public static String MD5(String src) {
        return MD5(src, "UTF-8");
    }

    public static String MD5(String src, String charset) {
        try {
            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] messageDigest = md.digest(src.getBytes(charset));
            BigInteger no = new BigInteger(1, messageDigest);
            StringBuilder sb = new StringBuilder(no.toString(16));
            while (sb.length() < 32) sb.insert(0, "0");
            return sb.toString().toLowerCase();
        } catch (Exception e) {
            return "";
        }
    }

    public static byte[] decompressGzip(byte[] compressed) throws IOException {
        try (java.io.ByteArrayInputStream bis = new java.io.ByteArrayInputStream(compressed);
             java.util.zip.GZIPInputStream gis = new java.util.zip.GZIPInputStream(bis)) {
            java.io.ByteArrayOutputStream bos = new java.io.ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            int n;
            while ((n = gis.read(buf)) >= 0) bos.write(buf, 0, n);
            return bos.toByteArray();
        }
    }

    @FunctionalInterface
    public interface CallBack {
        void apply(String val);
    }

    /** 从响应头提取 cookie（兼容 OkHttp toMultimap 的小写 set-cookie）。 */
    public static String cookiesFrom(OkResult result) {
        if (result == null) return "";
        List<String> cookies = result.getHeader("set-cookie");
        if (cookies == null || cookies.isEmpty()) return "";
        List<String> parts = new ArrayList<>();
        for (String cookie : cookies) {
            if (cookie == null || cookie.isEmpty()) continue;
            parts.add(cookie.split(";", 2)[0]);
        }
        return StringUtils.join(parts, ";");
    }

    /** Handshake timeouts for slow hosts (Win7). */
    public static final long UI_HANDSHAKE_MS = 20_000L;
    public static final long UI_CLOSE_WAIT_MS = 20_000L;

    /** 绑定当前线程的 Flutter clientId（一次 spider 调用期间有效）。 */
    public static void setClientId(String id) {
        if (id == null || id.isEmpty()) {
            CLIENT_ID_TL.remove();
        } else {
            CLIENT_ID_TL.set(id);
            // 供 jar 自起线程（无 ThreadLocal）拿网络配置时兜底。
            com.github.catvod.net.NetProfiles.touch(id);
        }
    }

    public static void clearClientId() {
        CLIENT_ID_TL.remove();
    }

    /** 当前线程绑定的 clientId；无则空串。可能是裸 id，也可能是已带 c:/u: 的 ScopeID。 */
    public static String clientId() {
        String s = CLIENT_ID_TL.get();
        return s == null ? "" : s;
    }

    /** 绑定远端 userId（裸 UUID，不含 u: 前缀）。 */
    public static void setUserId(String id) {
        if (id == null || id.isEmpty()) USER_ID_TL.remove();
        else USER_ID_TL.set(id.trim());
    }

    public static void clearUserId() {
        USER_ID_TL.remove();
    }

    public static String userId() {
        String s = USER_ID_TL.get();
        return s == null ? "" : s;
    }

    /**
     * 单 bridge 多用户可变态分桶键：远端裸 userId；本机空 uid → {@code shared}。
     * Prefers / spiders / recentJar 等一律走此键，勿再散落拼装。
     */
    public static String tenantKey() {
        String uid = userId().trim();
        if (uid.isEmpty()) return "shared";
        if (uid.startsWith("u:") && uid.length() > 2) return uid.substring(2).trim();
        if (uid.startsWith("c:")) return "shared"; // 本机 Scope 误写入 userId 时仍归共享桶
        return uid;
    }

    /** 绑定远端登录名（裸用户名）；仅 cookie 目录用。 */
    public static void setUsername(String name) {
        if (name == null || name.isEmpty()) USERNAME_TL.remove();
        else USERNAME_TL.set(name.trim());
    }

    public static void clearUsername() {
        USERNAME_TL.remove();
    }

    public static String username() {
        String s = USERNAME_TL.get();
        return s == null ? "" : s;
    }

    /**
     * 会话隔离键（UI/队列/遥控路由，不进 cookie 目录名）。
     * 远端租户 {@code u:<userId>}；本机多前端 {@code c:<clientId>}。
     */
    public static String scopeId() {
        String uid = userId();
        if (!uid.isEmpty()) {
            if (uid.startsWith("u:")) return uid;
            if (uid.startsWith("c:")) return uid; // 误写入则原样，勿再套 u:
            return "u:" + uid;
        }
        String cid = clientId();
        if (cid.isEmpty()) return "";
        if (cid.startsWith("u:") || cid.startsWith("c:")) return cid;
        return "c:" + cid;
    }

    /**
     * 站点凭证落盘键：本机 → 空；远端 → {@code u:<用户名>}。
     * {@link Path#tv} 只认这个，不认 userId。
     */
    public static String tvScopeId() {
        String name = username();
        if (name.isEmpty()) return "";
        return "u:" + name;
    }

    /**
     * {@code Path.tv} 子目录名片段（收拢于此；桌面/安卓 Path 只调本方法）。
     * 入参：{@code u:用户名} / {@code u_用户名} / 裸用户名。
     * 空或 {@code c:} → 空（落 {@code /TV/}）；否则返回 {@code u_用户名}
     * （Path 再加 {@code s_} → {@code /TV/s_u_用户名/}）。永不产生 {@code s_c_*}。
     */
    public static String tvDirName(String scopeOrName) {
        String name = tvUsernameOf(scopeOrName);
        if (name.isEmpty()) return "";
        return "u_" + name;
    }

    /** 从 scope/用户名抽出可落盘的用户名片段（非法路径字符替换为 _）。 */
    public static String tvUsernameOf(String scopeOrName) {
        if (scopeOrName == null) return "";
        String t = scopeOrName.trim();
        if (t.isEmpty() || t.startsWith("c:") || t.startsWith("c_")) return "";
        if (t.startsWith("u:")) t = t.substring(2);
        else if (t.startsWith("s_u_")) t = t.substring(4);
        else if (t.startsWith("u_")) t = t.substring(2);
        return sanitizeTvUsername(t);
    }

    private static String sanitizeTvUsername(String name) {
        if (name == null) return "";
        StringBuilder b = new StringBuilder(name.length());
        for (int i = 0; i < name.length(); ) {
            int cp = name.codePointAt(i);
            i += Character.charCount(cp);
            if (cp == '/' || cp == '\\' || cp == ':' || cp == '*' || cp == '?'
                    || cp == '"' || cp == '<' || cp == '>' || cp == '|'
                    || cp == '#' || cp == '$') {
                b.append('_');
                continue;
            }
            if (Character.isISOControl(cp)) continue;
            b.appendCodePoint(cp);
        }
        String out = b.toString().trim();
        while (out.startsWith(".")) out = out.substring(1).trim();
        // 按 code point 截到 64，与 Go paths.sanitizeTVUsername 一致。
        int cps = out.codePointCount(0, out.length());
        if (cps > 64) {
            out = out.substring(0, out.offsetByCodePoints(0, 64)).trim();
        }
        return out.trim();
    }

    public static void clearScope() {
        clearClientId();
        clearUserId();
        clearUsername();
    }

    /** 远端客户端走安卓 jar 时 sticky：AlertDialog.show() 常 post 到主线程，ThreadLocal 会丢。 */
    private static final java.util.concurrent.atomic.AtomicInteger REMOTE_UI_DEPTH =
            new java.util.concurrent.atomic.AtomicInteger();
    private static volatile String REMOTE_UI_CLIENT = "";
    private static volatile String REMOTE_UI_USER = "";
    private static volatile String REMOTE_UI_USERNAME = "";

    public static void enterRemoteUi(String clientId, String userId, String username) {
        REMOTE_UI_CLIENT = clientId == null ? "" : clientId;
        REMOTE_UI_USER = userId == null ? "" : userId;
        REMOTE_UI_USERNAME = username == null ? "" : username.trim();
        REMOTE_UI_DEPTH.incrementAndGet();
    }

    public static void leaveRemoteUi() {
        if (REMOTE_UI_DEPTH.decrementAndGet() < 0) {
            REMOTE_UI_DEPTH.set(0);
        }
        // 不立刻清 clientId：dialog.show() 可能还在主线程队列里。
    }

    /** 本机调用开始时清掉上一次远端标记，避免本地弹窗被误中继。 */
    public static void clearRemoteUi() {
        if (REMOTE_UI_DEPTH.get() <= 0) {
            REMOTE_UI_CLIENT = "";
            REMOTE_UI_USER = "";
            REMOTE_UI_USERNAME = "";
            REMOTE_UI_DEPTH.set(0);
        }
    }

    public static boolean hasRemoteUi() {
        String id = REMOTE_UI_CLIENT;
        return id != null && !id.isEmpty();
    }

    public static String remoteClientId() {
        String s = REMOTE_UI_CLIENT;
        return s == null ? "" : s;
    }

    public static String remoteUserId() {
        String s = REMOTE_UI_USER;
        return s == null ? "" : s;
    }

    public static String remoteUsername() {
        String s = REMOTE_UI_USERNAME;
        return s == null ? "" : s;
    }

    /**
     * Notify host via the UI socket; also logs.
     * {@code UI:}/{@code UI_CLOSE:} are sent synchronously so waitUiAction can start after delivery.
     * Toasts stay async on the single-thread queue.
     */
    public static void notify(String msg) {
        if (msg == null || msg.isEmpty()) return;
        if (msg.startsWith("UI:") || msg.startsWith("UI_CLOSE:")) {
            notifySync(msg);
            return;
        }
        SpiderDebug.log(msg);
        long epoch = UI_NOTIFY_EPOCH.get();
        // 异步队列在另一线程执行，必须捕获当前 Scope，否则会丢路由。
        final String cid = clientId();
        final String uid = userId();
        final String uname = username();
        UI_MESSAGES.execute(() -> {
            if (epoch != UI_NOTIFY_EPOCH.get()) {
                SpiderDebug.log("界面消息已丢弃");
                return;
            }
            try {
                setClientId(cid);
                setUserId(uid);
                setUsername(uname);
                sendUi(msg);
            } catch (Exception e) {
                SpiderDebug.log("界面消息发送失败: " + e.getClass().getSimpleName() + ": " + e.getMessage());
            } finally {
                clearScope();
            }
        });
    }

    /** 经界面套接字发出。shown/closed 由对端帧唤醒，不另开通道。 */
    public static void notifySync(String msg) {
        if (msg == null || msg.isEmpty()) return;
        SpiderDebug.log(msg.length() > 240 ? msg.substring(0, 240) + "…(" + msg.length() + ")" : msg);
        try {
            // 池线程里若刚被 shutdownNow 中断，清掉标志再发，避免 OkHttp 直接失败且 message=null
            Thread.interrupted();
            sendUi(msg);
        } catch (Exception e) {
            SpiderDebug.log("界面消息发送失败: " + e.getClass().getSimpleName() + ": " + e.getMessage());
        }
    }

    /**
     * Drop queued async toast {@link #notify} payloads.
     * Do not call between sending {@code UI_CLOSE:} and waiting for {@code closed}.
     */
    public static void clearPendingUiNotify() {
        UI_NOTIFY_EPOCH.incrementAndGet();
    }

    /**
     * 堵住直到这个 session 的下一条业务帧，或超时。套接字入队的线程会唤醒。
     * 超时或中断返回空串。{@code shown} 不在这条队列里。
     */
    public static String awaitUiReplyRaw(String session, long timeoutMs) {
        return awaitFrame(session, timeoutMs, false);
    }

    /** Parse action field from a raw uiReply body. */
    public static String uiReplyAction(String raw) {
        if (raw == null || raw.isEmpty()) return "";
        String body = raw.trim();
        if (body.startsWith("{")) {
            Map<String, Object> event = Json.parseSafe(body, Map.class);
            if (event == null) return "";
            return String.valueOf(event.getOrDefault("action", "")).trim();
        }
        return body;
    }

    /**
     * 按弹窗 session 记住宿主客户端平台（android/ios/…）。
     * 多前端连同一引擎时不能用全局「最近一次」——会串台。
     */
    private static final java.util.concurrent.ConcurrentHashMap<String, String> hostPlatformBySession =
            new java.util.concurrent.ConcurrentHashMap<>();
    private static final java.util.concurrent.ConcurrentHashMap<String, String> hostOsBySession =
            new java.util.concurrent.ConcurrentHashMap<>();
    private static final java.util.concurrent.ConcurrentHashMap<String, Boolean> hostDesktopBySession =
            new java.util.concurrent.ConcurrentHashMap<>();

    /** @param session 弹窗 id；空则返回空串（勿再依赖全局最近一次） */
    public static String hostPlatform(String session) {
        if (session == null || session.isEmpty()) return "";
        String p = hostPlatformBySession.get(session);
        return p == null ? "" : p;
    }

    public static String hostOs(String session) {
        if (session == null || session.isEmpty()) return "";
        String os = hostOsBySession.get(session);
        return os == null ? "" : os;
    }

    public static boolean hostDesktop(String session) {
        if (session == null || session.isEmpty()) return false;
        return Boolean.TRUE.equals(hostDesktopBySession.get(session));
    }

    /** @deprecated 多前端会串台；请用 {@link #hostPlatform(String session)} 或 {@link UiBridge#hostPlatform(String kind)} */
    @Deprecated
    public static String hostPlatform() {
        return "";
    }

    /** @deprecated 多前端会串台；请用 {@link #hostDesktop(String session)} */
    @Deprecated
    public static boolean hostDesktop() {
        return false;
    }

    @SuppressWarnings("unchecked")
    private static void rememberHostFromReply(String session, String raw) {
        if (session == null || session.isEmpty()) return;
        if (raw == null || raw.isEmpty() || !raw.trim().startsWith("{")) return;
        Map<String, Object> event = Json.parseSafe(raw.trim(), Map.class);
        if (event == null) return;
        Object valuesObj = event.get("values");
        if (!(valuesObj instanceof Map)) return;
        Map<?, ?> values = (Map<?, ?>) valuesObj;
        Object platform = values.get("platform");
        if (platform != null) {
            String p = String.valueOf(platform).trim().toLowerCase(Locale.ROOT);
            if (!p.isEmpty() && !"null".equals(p)) hostPlatformBySession.put(session, p);
        }
        Object desktop = values.get("desktop");
        if (desktop != null) {
            String d = String.valueOf(desktop).trim().toLowerCase(Locale.ROOT);
            hostDesktopBySession.put(session, "true".equals(d) || "1".equals(d) || "yes".equals(d));
        }
        Object os = values.get("os");
        if (os != null) {
            String value = String.valueOf(os).trim().toLowerCase(Locale.ROOT);
            if (!value.isEmpty() && !"null".equals(value) && !"unknown".equals(value)) {
                hostOsBySession.put(session, value);
            }
        }
    }

    static void clearHostClientInfo(String session) {
        if (session == null || session.isEmpty()) return;
        hostPlatformBySession.remove(session);
        hostOsBySession.remove(session);
        hostDesktopBySession.remove(session);
    }

    /**
     * Block until host reports {@code want} (or a compatible terminal action) for {@code session}.
     * Used for shown/closed handshake; does not dispatch business cancel/submit handlers.
     * When waiting for {@code shown}, caches platform under this {@code session}.
     */
    public static boolean waitUiAction(String session, String want, long timeoutMs) {
        if (session == null || session.isEmpty() || want == null || want.isEmpty()) return false;
        long deadline = System.currentTimeMillis() + Math.max(200L, timeoutMs);
        String expect = want.trim().toLowerCase(Locale.ROOT);
        while (System.currentTimeMillis() < deadline) {
            long left = deadline - System.currentTimeMillis();
            if (left <= 0) break;
            String raw = awaitFrame(session, left, true);
            String action = uiReplyAction(raw).toLowerCase(Locale.ROOT);
            if (action.isEmpty()) return false;
            if (action.equals(expect)) {
                if ("shown".equals(expect)) rememberHostFromReply(session, raw);
                if ("closed".equals(expect)) clearHostClientInfo(session);
                return true;
            }
            if ("shown".equals(expect)) {
                if (isUiTerminalAction(action)) return false;
                continue;
            }
            if ("closed".equals(expect) && isUiTerminalAction(action)) {
                clearHostClientInfo(session);
                return true;
            }
        }
        return false;
    }

    private static boolean isUiTerminalAction(String action) {
        return "closed".equals(action)
                || "dismiss".equals(action)
                || "timeout".equals(action);
    }

    /**
     * 堵住直到这个 kind 的下一条业务回复。{@code shown} 只进开关握手。
     * 没有这个 kind 的窗口、超时或中断都返回空串。
     */
    public static String awaitUiReply(String kind, long timeoutMs) {
        if (kind == null || kind.isEmpty()) return "";
        try {
            String session = UiBridge.currentSession(kind);
            if (session.isEmpty()) return "";
            return decodeUiReply(awaitUiReplyRaw(session, Math.max(1L, timeoutMs)));
        } catch (Exception e) {
            return "";
        }
    }

    private static String decodeUiReply(String raw) {
        if (raw == null || raw.isEmpty()) return "";
        String action;
        Map<String, Object> event = null;
        if (raw.startsWith("{")) {
            event = Json.parseSafe(raw, Map.class);
            if (event == null) return "";
            action = String.valueOf(event.getOrDefault("action", "")).trim();
        } else {
            action = raw.trim();
        }
        if ("shown".equalsIgnoreCase(action)) return "";
        if ("closed".equals(action)) return "CLOSED";
        if ("dismiss".equals(action) || "timeout".equals(action)) return "CANCEL";
        if ("submit".equals(action)) {
            Object value = null;
            if (event != null) {
                Object valuesObj = event.get("values");
                if (valuesObj instanceof Map) {
                    value = ((Map<?, ?>) valuesObj).get("value");
                }
            }
            return value == null ? "SUBMIT:" : "SUBMIT:" + value;
        }
        if ("cancel".equals(action)) return "CANCEL";
        if (!action.isEmpty()) return action.toUpperCase(Locale.ROOT);
        return "";
    }

    private static final Object UI_SOCKET_LOCK = new Object();

    /** 同一帧拆成两份：握手读 life，按钮回复读 biz，避免互相拿走后另一边空等。 */
    private static final class UiFrames {
        final ArrayDeque<String> life = new ArrayDeque<>();
        final ArrayDeque<String> biz = new ArrayDeque<>();
    }

    private static final ConcurrentHashMap<String, UiFrames> UI_REPLY_Q = new ConcurrentHashMap<>();
    private static volatile WebSocket UI_SOCKET;
    private static volatile OkHttpClient UI_SOCKET_CLIENT;

    private static OkHttpClient uiSocketClient() {
        OkHttpClient existing = UI_SOCKET_CLIENT;
        if (existing != null) return existing;
        synchronized (UI_SOCKET_LOCK) {
            if (UI_SOCKET_CLIENT == null) {
                UI_SOCKET_CLIENT = OkHttp.client().newBuilder()
                        .readTimeout(0, TimeUnit.MILLISECONDS)
                        .pingInterval(30, TimeUnit.SECONDS)
                        .build();
            }
            return UI_SOCKET_CLIENT;
        }
    }

    private static void ensureUiSocket() {
        if (UI_SOCKET != null) return;
        synchronized (UI_SOCKET_LOCK) {
            openUiSocketLocked();
        }
    }

    private static void openUiSocketLocked() {
        if (UI_SOCKET != null) return;
        String base = Proxy.getHostPort();
        if (base == null || !base.startsWith("http")) return;
        String url = "ws" + base.substring("http".length()) + "/ui?role=src";
        Request req = new Request.Builder().url(url).build();
        UI_SOCKET = uiSocketClient().newWebSocket(req, new WebSocketListener() {
            @Override
            public void onMessage(WebSocket webSocket, String text) {
                enqueueUiReply(text);
            }

            @Override
            public void onClosing(WebSocket webSocket, int code, String reason) {
                webSocket.close(code, null);
            }

            @Override
            public void onClosed(WebSocket webSocket, int code, String reason) {
                clearUiSocket(webSocket);
            }

            @Override
            public void onFailure(WebSocket webSocket, Throwable t, Response response) {
                clearUiSocket(webSocket);
            }
        });
    }

    private static void clearUiSocket(WebSocket webSocket) {
        synchronized (UI_SOCKET_LOCK) {
            if (UI_SOCKET == webSocket) UI_SOCKET = null;
        }
    }

    private static void enqueueUiReply(String text) {
        if (text == null) return;
        text = text.trim();
        if (text.isEmpty() || !text.startsWith("{")) return;
        String id;
        try {
            id = new JSONObject(text).optString("id", "").trim();
        } catch (Exception e) {
            return;
        }
        if (id.isEmpty()) return;
        String action = uiReplyAction(text);
        UiFrames box = UI_REPLY_Q.computeIfAbsent(id, key -> new UiFrames());
        synchronized (box) {
            if (isHandshakeFrame(action)) box.life.addLast(text);
            if (!"shown".equalsIgnoreCase(action)) box.biz.addLast(text);
            box.notifyAll();
        }
    }

    private static boolean isHandshakeFrame(String action) {
        return "shown".equalsIgnoreCase(action)
                || "closed".equalsIgnoreCase(action)
                || "dismiss".equalsIgnoreCase(action)
                || "timeout".equalsIgnoreCase(action);
    }

    private static String awaitFrame(String session, long timeoutMs, boolean handshake) {
        if (session == null || session.isEmpty()) return "";
        ensureUiSocket();
        long deadline = System.currentTimeMillis() + Math.max(1L, timeoutMs);
        UiFrames box = UI_REPLY_Q.computeIfAbsent(session, key -> new UiFrames());
        synchronized (box) {
            ArrayDeque<String> q = handshake ? box.life : box.biz;
            while (true) {
                if (!q.isEmpty()) return q.pollFirst();
                long left = deadline - System.currentTimeMillis();
                if (left <= 0) return "";
                try {
                    box.wait(left);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    return "";
                }
            }
        }
    }

    private static void sendUi(String msg) throws IOException {
        String uid = userId();
        if (uid.startsWith("u:")) uid = uid.substring(2);
        if (uid.startsWith("c:")) uid = "";
        String cid = clientId();
        String userField = "";
        String clientField = "";
        String scopeField = "";
        if (!uid.isEmpty()) {
            userField = uid;
        } else if (!cid.isEmpty()) {
            if (cid.startsWith("c:")) {
                clientField = cid.substring(2);
            } else if (cid.startsWith("u:")) {
                userField = cid.substring(2);
            } else {
                clientField = cid;
            }
        } else {
            scopeField = scopeId();
        }
        StringBuilder payload = new StringBuilder(msg.length() + 64);
        payload.append("{\"msg\":").append(JSONObject.quote(msg));
        if (!userField.isEmpty()) payload.append(",\"userId\":").append(JSONObject.quote(userField));
        if (!clientField.isEmpty()) payload.append(",\"clientId\":").append(JSONObject.quote(clientField));
        if (!scopeField.isEmpty()) payload.append(",\"scopeId\":").append(JSONObject.quote(scopeField));
        payload.append('}');
        String text = payload.toString();
        synchronized (UI_SOCKET_LOCK) {
            openUiSocketLocked();
            WebSocket ws = UI_SOCKET;
            if (ws != null && ws.send(text)) return;
            if (ws != null) {
                ws.cancel();
                UI_SOCKET = null;
            }
            openUiSocketLocked();
            ws = UI_SOCKET;
            if (ws == null || !ws.send(text)) {
                throw new IOException("ui socket");
            }
        }
    }

    public static void notify(String msg, Integer timeMills) {
        notify(msg);
    }

    public static void showToast(String msg, Integer timeMills) {
        notify(msg == null ? "" : msg);
    }

    public static String getDigit(String text) {
        try {
            String newText = text;
            Matcher matcher = Pattern.compile(".*(1080|720|2160|4k|4K).*").matcher(text);
            if (matcher.find()) newText = matcher.group(1) + " " + text;
            matcher = Pattern.compile("^([0-9]+)").matcher(text);
            if (matcher.find()) newText = matcher.group(1) + " " + newText;
            return newText.replaceAll("\\D+", "") + " " + newText.replaceAll("\\d+", "");
        } catch (Exception e) {
            return "";
        }
    }

    public static String getMimeType(String contentDisposition) {
        if (contentDisposition.endsWith(".mp4")) {
            return "video/mp4";
        } else if (contentDisposition.endsWith(".webm")) {
            return "video/webm";
        } else if (contentDisposition.endsWith(".avi")) {
            return "video/x-msvideo";
        } else if (contentDisposition.endsWith(".wmv")) {
            return "video/x-ms-wmv";
        } else if (contentDisposition.endsWith(".flv")) {
            return "video/x-flv";
        } else if (contentDisposition.endsWith(".mov")) {
            return "video/quicktime";
        } else if (contentDisposition.endsWith(".mkv")) {
            return "video/x-matroska";
        } else if (contentDisposition.endsWith(".mpeg")) {
            return "video/mpeg";
        } else if (contentDisposition.endsWith(".3gp")) {
            return "video/3gpp";
        } else if (contentDisposition.endsWith(".ts")) {
            return "video/MP2T";
        } else if (contentDisposition.endsWith(".mp3")) {
            return "audio/mp3";
        } else if (contentDisposition.endsWith(".wav")) {
            return "audio/wav";
        } else if (contentDisposition.endsWith(".aac")) {
            return "audio/aac";
        } else {
            return null;
        }
    }

    public static void sleep(Integer time) {
        try {
            Thread.sleep(time);
        } catch (InterruptedException e) {
//            throw new RuntimeException(e);
        }
    }

    /**
     * @param referer
     * @param cookie  多个cookie name=value;name2=value2
     * @return
     */
    public static HashMap<String, String> webHeaders(String referer, String cookie) {
        return webHeaders(referer, "", cookie);
    }

    public static HashMap<String, String> webHeaders(String referer) {
        return webHeaders(referer, "");
    }

    public static HashMap<String, String> webHeaders(String referer, String url, String cookie) {
        webHttpHeaderMap = new HashMap<>();
        webHttpHeaderMap.put("Accept-Language", "zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2");
        webHttpHeaderMap.put("Connection", "keep-alive");
        webHttpHeaderMap.put("User-Agent", CHROME);
        webHttpHeaderMap.put("Accept", "*/*");
        if (StringUtils.isNotBlank(referer)) {
            webHttpHeaderMap.put("Referer", referer);
        }
        // Host 由 HTTP 栈按请求地址填写。这里写页面主机后，同一张表若被挂到 CDN 播放地址上会 404。
        if (StringUtils.isNotBlank(cookie)) {
            webHttpHeaderMap.put("Cookie", cookie);
        }
        return webHttpHeaderMap;
    }

    public static String timestampToDateStr(Long timestamp) {
        return new SimpleDateFormat("EEE, dd MMM yyyy HH:mm:ss z", Locale.US).format(new Date(timestamp));
    }

    public static String base64Encode(String str) {
        if (str == null) return "";
        return new String(Base64.getEncoder().encode(str.getBytes()));
    }


    public static String base64Encode(byte[] str) {
        return new String(Base64.getEncoder().encode(str));
    }

    public static String base64Decode(String str) {
        if (StringUtils.isBlank(str)) return "";
        try {
            return new String(Base64.getDecoder().decode(str));
        } catch (IllegalArgumentException e) {
            // 配置偶发把明文 URL/JSON 当 Base64；避免整站 init 直接崩溃。
            return "";
        }
    }

    public static String stringJoin(String separate, Collection<String> list) {
        return StringUtils.join(list, separate);
    }

    public static String stringJoin(Collection<String> list, String separate) {
        return StringUtils.join(list, separate);
    }

    /**
     * 字符串相似度匹配
     *
     * @returns
     */

    public static LCSResult lcs(String str1, String str2) {
        if (str1 == null || str2 == null) {
            return new LCSResult(0, "", 0);
        }

        StringBuilder sequence = new StringBuilder();
        int str1Length = str1.length();
        int str2Length = str2.length();
        int[][] num = new int[str1Length][str2Length];
        int maxlen = 0;
        int lastSubsBegin = 0;

        for (int i = 0; i < str1Length; i++) {
            for (int j = 0; j < str2Length; j++) {
                if (str1.charAt(i) != str2.charAt(j)) {
                    num[i][j] = 0;
                } else {
                    if (i == 0 || j == 0) {
                        num[i][j] = 1;
                    } else {
                        num[i][j] = 1 + num[i - 1][j - 1];
                    }

                    if (num[i][j] > maxlen) {
                        maxlen = num[i][j];
                        int thisSubsBegin = i - num[i][j] + 1;
                        if (lastSubsBegin == thisSubsBegin) {
                            // if the current LCS is the same as the last time this block ran
                            sequence.append(str1.charAt(i));
                        } else {
                            // this block resets the string builder if a different LCS is found
                            lastSubsBegin = thisSubsBegin;
                            sequence.setLength(0); // clear it
                            sequence.append(str1.substring(lastSubsBegin, i + 1));
                        }
                    }
                }
            }
        }
        return new LCSResult(maxlen, sequence.toString(), lastSubsBegin);
    }

    public static class LCSResult {
        public int length;
        public String sequence;
        public int offset;

        public LCSResult(int length, String sequence, int offset) {
            this.length = length;
            this.sequence = sequence;
            this.offset = offset;
        }
    }

    public static Integer findAllIndexes(List<String> arr, String value) {

        for (int i = 0; i < arr.size(); i++) {
            if (arr.get(i).equals(value)) {
                return i;
            }
        }
        return 0;
    }


    public static final Pattern THUNDER = Pattern.compile("(magnet|thunder|ed2k):.*");

    public static boolean isThunder(String url) {
        return THUNDER.matcher(url).find() || isTorrent(url);
    }

    public static boolean isTorrent(String url) {
        return !url.startsWith("magnet") && url.endsWith(".torrent");
    }

    public static void copy(String text) {
        try {
            android.content.ClipboardManager manager = (android.content.ClipboardManager) Init.context().getSystemService(android.content.Context.CLIPBOARD_SERVICE);
            manager.setPrimaryClip(android.content.ClipData.newPlainText("kotv", text));
            notify("已复制");
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    // --- host ABI（bridge 原有，站点瘦包后父优先落到这里）---

    public static final String OKHTTP = "okhttp/" + okhttp3.OkHttp.VERSION;
    public static final int URL_SAFE = android.util.Base64.DEFAULT | android.util.Base64.URL_SAFE | android.util.Base64.NO_WRAP;

    public static String base64(String s) {
        return base64(s.getBytes(StandardCharsets.UTF_8));
    }

    public static String base64(byte[] bytes) {
        return base64(bytes, android.util.Base64.DEFAULT | android.util.Base64.NO_WRAP);
    }

    public static String base64(String s, int flags) {
        return base64(s.getBytes(StandardCharsets.UTF_8), flags);
    }

    public static String base64(byte[] bytes, int flags) {
        return android.util.Base64.encodeToString(bytes, flags);
    }

    public static byte[] decode(String s) {
        return decode(s, android.util.Base64.DEFAULT | android.util.Base64.NO_WRAP);
    }

    public static byte[] decode(String s, int flags) {
        return android.util.Base64.decode(s, flags);
    }

    public static byte[] hex2byte(String s) {
        byte[] bytes = new byte[s.length() / 2];
        for (int i = 0; i < bytes.length; i++) {
            bytes[i] = Integer.valueOf(s.substring(i * 2, i * 2 + 2), 16).byteValue();
        }
        return bytes;
    }

    public static boolean equals(String name, String md5) {
        return md5(Path.jar(name)).equalsIgnoreCase(md5);
    }

    public static String md5(String src) {
        return MD5(src);
    }

    public static String md5(java.io.File file) {
        try {
            MessageDigest digest = MessageDigest.getInstance("MD5");
            try (java.io.FileInputStream fis = new java.io.FileInputStream(file)) {
                byte[] bytes = new byte[16384];
                int count;
                while ((count = fis.read(bytes)) != -1) digest.update(bytes, 0, count);
            }
            StringBuilder sb = new StringBuilder();
            for (byte b : digest.digest()) sb.append(Integer.toString((b & 0xff) + 0x100, 16).substring(1));
            return sb.toString();
        } catch (Exception e) {
            return "";
        }
    }

    public static boolean containOrMatch(String text, String regex) {
        try {
            return text.contains(regex) || text.matches(regex);
        } catch (Exception e) {
            return false;
        }
    }

    public static String getIp() {
        try {
            String ip = getHostAddress("wlan");
            if (!ip.isEmpty()) return ip;
            ip = getHostAddress("eth");
            if (!ip.isEmpty()) return ip;
            ip = getWifiAddress();
            if (!ip.isEmpty()) return ip;
            return getHostAddress("");
        } catch (Exception e) {
            return "";
        }
    }

    /** Android WifiManager 回落；桌面无该类则返回空。 */
    private static String getWifiAddress() {
        try {
            Object ctx = Init.context();
            if (ctx == null) return "";
            Class<?> ctxClz = Class.forName("android.content.Context");
            Object app = ctxClz.getMethod("getApplicationContext").invoke(ctx);
            Object manager = ctxClz.getMethod("getSystemService", String.class).invoke(app, "wifi");
            if (manager == null) return "";
            Object info = manager.getClass().getMethod("getConnectionInfo").invoke(manager);
            int ip = ((Integer) info.getClass().getMethod("getIpAddress").invoke(info)).intValue();
            if (ip == 0) return "";
            return String.format(java.util.Locale.getDefault(), "%d.%d.%d.%d",
                    ip & 0xFF, (ip >> 8) & 0xFF, (ip >> 16) & 0xFF, (ip >> 24) & 0xFF);
        } catch (Throwable ignored) {
            return "";
        }
    }

    private static String getHostAddress(String keyword) throws java.net.SocketException {
        for (java.util.Enumeration<java.net.NetworkInterface> en = java.net.NetworkInterface.getNetworkInterfaces(); en.hasMoreElements(); ) {
            java.net.NetworkInterface nif = en.nextElement();
            if (!keyword.isEmpty() && !nif.getName().startsWith(keyword)) continue;
            for (java.util.Enumeration<java.net.InetAddress> addresses = nif.getInetAddresses(); addresses.hasMoreElements(); ) {
                java.net.InetAddress addr = addresses.nextElement();
                if (!addr.isLoopbackAddress() && addr instanceof java.net.Inet4Address) {
                    return addr.getHostAddress();
                }
            }
        }
        return "";
    }

    private static String urlEncode(String s) {
        try {
            return java.net.URLEncoder.encode(s == null ? "" : s, "UTF-8");
        } catch (Exception e) {
            return s == null ? "" : s;
        }
    }
}
