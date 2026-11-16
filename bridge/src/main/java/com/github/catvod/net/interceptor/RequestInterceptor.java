package com.github.catvod.net.interceptor;

import androidx.annotation.NonNull;

import com.github.catvod.utils.Util;

import java.io.IOException;
import java.util.Iterator;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

import okhttp3.HttpUrl;
import okhttp3.Interceptor;
import okhttp3.Request;
import okhttp3.Response;

public class RequestInterceptor implements Interceptor {

    /** 键：tenantKey + "\\0" + host */
    private final ConcurrentHashMap<String, String> authMap;

    public RequestInterceptor() {
        authMap = new ConcurrentHashMap<>();
    }

    public void clear() {
        authMap.clear();
    }

    /** 只清某一租户的 auth 记忆（KillUser / clearTenant）。 */
    public void clearTenant(String tenant) {
        purgeTenantPrefix(authMap, tenant);
    }

    @NonNull
    @Override
    public Response intercept(@NonNull Chain chain) throws IOException {
        Request request = chain.request();
        if ("1".equals(request.header("X-KOTV-Invalid-Url"))) {
            throw new IOException("invalid url");
        }
        Request.Builder builder = request.newBuilder();
        // 自动带上当前 JAR 调用的 Flutter clientId，供 OkHttp.cancel(clientId) 软取消。
        if (request.tag() == null) {
            try {
                String cid = Util.clientId();
                if (cid != null && !cid.isEmpty()) {
                    builder.tag(cid);
                }
            } catch (Throwable ignored) {
            }
        }
        HttpUrl url = request.url();
        checkAuth(url, builder);
        return chain.proceed(builder.build());
    }

    private void checkAuth(HttpUrl url, Request.Builder builder) {
        String host = url.host();
        String key = tenantHostKey(host);
        String auth = url.queryParameter("auth");
        if (auth != null) authMap.put(key, auth);
        else if (authMap.containsKey(key)) {
            builder.url(url.newBuilder().addQueryParameter("auth", authMap.get(key)).build());
        }
    }

    public static String tenantHostKey(String host) {
        return currentTenant() + "\0" + (host == null ? "" : host);
    }

    static String currentTenant() {
        try {
            String t = Util.tenantKey();
            if (t != null && !t.isEmpty()) return t;
        } catch (Throwable ignored) {
        }
        return "shared";
    }

    static void purgeTenantPrefix(ConcurrentHashMap<String, String> map, String tenant) {
        if (map == null) return;
        if (tenant == null || tenant.isEmpty()) tenant = "shared";
        String prefix = tenant + "\0";
        for (Iterator<Map.Entry<String, String>> it = map.entrySet().iterator(); it.hasNext(); ) {
            Map.Entry<String, String> e = it.next();
            String k = e.getKey();
            if (k != null && k.startsWith(prefix)) it.remove();
        }
    }
}
