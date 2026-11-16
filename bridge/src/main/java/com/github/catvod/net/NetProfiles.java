package com.github.catvod.net;

import com.github.catvod.bean.Doh;
import com.github.catvod.bean.Header;
import com.github.catvod.bean.Proxy;

import java.util.Collections;
import java.util.Iterator;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * 按 ScopeID（clientId，形如 u:&lt;uid&gt; / c:&lt;id&gt;）隔离点播 headers / proxy / hosts / doh。
 * 空 key 为默认桶（本机 shared 兼容）。
 * 无绑定线程只回落默认桶，禁止跨租户 lastActive 兜底。
 */
public final class NetProfiles {

    public static final class Profile {
        public final List<Header> headers = new CopyOnWriteArrayList<>();
        public final List<Proxy> proxies = new CopyOnWriteArrayList<>();
        public final ConcurrentHashMap<String, String> hosts = new ConcurrentHashMap<>();
        public volatile Doh doh;
    }

    private static final ConcurrentHashMap<String, Profile> BY_CLIENT = new ConcurrentHashMap<>();

    private NetProfiles() {}

    public static String key(String clientId) {
        return clientId == null ? "" : clientId.trim();
    }

    /** SpiderBridge 绑定 clientId 时调用（诊断/扩展点；get 不再靠跨租户兜底）。 */
    public static void touch(String clientId) {
        key(clientId);
    }

    public static Profile get(String clientId) {
        String k = key(clientId);
        Profile p = BY_CLIENT.get(k);
        if (p != null) return p;
        // 无绑定或未知 Scope：只回落默认桶，禁止串到其它用户。
        return BY_CLIENT.computeIfAbsent("", x -> new Profile());
    }

    public static Profile put(String clientId) {
        return BY_CLIENT.computeIfAbsent(key(clientId), x -> new Profile());
    }

    public static void replace(String clientId, List<Header> headers, List<Proxy> proxies,
                               Map<String, String> hosts, Doh doh) {
        Profile p = put(clientId);
        p.headers.clear();
        if (headers != null && !headers.isEmpty()) p.headers.addAll(headers);
        p.proxies.clear();
        if (proxies != null && !proxies.isEmpty()) {
            proxies.forEach(Proxy::init);
            p.proxies.addAll(proxies);
            p.proxies.sort(null);
        }
        p.hosts.clear();
        if (hosts != null && !hosts.isEmpty()) p.hosts.putAll(hosts);
        p.doh = doh;
    }

    public static void remove(String clientId) {
        BY_CLIENT.remove(key(clientId));
    }

    /** 清某租户：shared→默认桶；远端→裸 id 与 u:&lt;id&gt;。 */
    public static void removeTenant(String tenant) {
        if (tenant == null || tenant.isEmpty()) tenant = "shared";
        String t = tenant.trim();
        if ("shared".equals(t)) {
            BY_CLIENT.remove("");
            return;
        }
        BY_CLIENT.remove(t);
        BY_CLIENT.remove("u:" + t);
        String prefix = "u:" + t;
        for (Iterator<String> it = BY_CLIENT.keySet().iterator(); it.hasNext();) {
            String cid = it.next();
            if (cid != null && cid.startsWith(prefix) && cid.length() > prefix.length()) {
                char n = cid.charAt(prefix.length());
                if (n == ':' || n == '/') it.remove();
            }
        }
    }

    public static void clearAll() {
        BY_CLIENT.clear();
    }

    public static Map<String, Profile> snapshot() {
        return Collections.unmodifiableMap(BY_CLIENT);
    }
}
