package com.bobo.kotv.bridge;

import com.github.catvod.Init;
import com.github.catvod.net.NetProfiles;
import com.github.catvod.net.OkHttp;
import com.github.catvod.net.interceptor.RequestInterceptor;
import com.github.catvod.utils.Prefers;
import com.github.catvod.utils.Util;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.File;
import java.lang.reflect.Method;
import java.nio.file.Files;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * 单 JVM 多租户：Prefers / files / tenantKey 交错不串；clearTenant 可反射调用。
 */
class TenantIsolationTest {

    private File root;
    private android.content.Context ctx;

    @BeforeEach
    void setUp() throws Exception {
        root = Files.createTempDirectory("kotv-tenant").toFile();
        File cache = new File(root, "cache");
        File run = new File(root, "run");
        cache.mkdirs();
        run.mkdirs();
        System.setProperty("kotv.cache.dir", cache.getAbsolutePath());
        System.setProperty("kotv.user.runtime.root", run.getAbsolutePath());
        System.clearProperty("kotv.user.dir");
        ctx = new android.content.Context();
        Init.set(ctx);
    }

    @AfterEach
    void tearDown() {
        Util.clearScope();
        deleteTree(root);
    }

    @Test
    void prefersAndFilesStayPerTenant() throws Exception {
        Util.setUserId("alice");
        Util.setClientId("u:alice");
        assertEquals("alice", Util.tenantKey());
        Prefers.put("token", "alice-token");
        File filesA = ctx.getFilesDir();

        Util.setUserId("bob");
        Util.setClientId("u:bob");
        assertEquals("bob", Util.tenantKey());
        Prefers.put("token", "bob-token");
        File filesB = ctx.getFilesDir();

        assertFalse(filesA.getAbsolutePath().equals(filesB.getAbsolutePath()));
        assertTrue(filesA.getAbsolutePath().contains(File.separator + "alice" + File.separator));
        assertTrue(filesB.getAbsolutePath().contains(File.separator + "bob" + File.separator));

        Util.setUserId("alice");
        assertEquals("alice-token", Prefers.getString("token"));
        Util.setUserId("bob");
        assertEquals("bob-token", Prefers.getString("token"));

        Util.setUserId("");
        assertEquals("shared", Util.tenantKey());

        NetProfiles.replace("u:alice", List.of(), List.of(), java.util.Map.of("h", "1"), null);
        NetProfiles.replace("u:bob", List.of(), List.of(), java.util.Map.of("h", "2"), null);
        assertFalse(NetProfiles.get("u:alice").hosts.isEmpty());

        Method clearTenant = SpiderBridge.class.getDeclaredMethod("clearTenant", String.class);
        clearTenant.setAccessible(true);
        clearTenant.invoke(null, "alice");
        assertTrue(NetProfiles.snapshot().get("u:alice") == null);
        assertFalse(NetProfiles.snapshot().get("u:bob") == null);
        clearTenant.invoke(null, "bob");

        // 无 clientId 不得串到其它租户桶
        Util.clearScope();
        assertTrue(NetProfiles.get("").hosts.isEmpty() || NetProfiles.get("").hosts.get("h") == null
                || !("1".equals(NetProfiles.get("").hosts.get("h"))));
    }

    @Test
    void authMapsStayPerTenantAndClearTenantPurges() throws Exception {
        RequestInterceptor req = OkHttp.requestInterceptor();
        Util.setUserId("alice");
        Util.setClientId("u:alice");
        String aliceKey = RequestInterceptor.tenantHostKey("example.com");
        // 经反射写入，避免真发 HTTP。
        java.lang.reflect.Field f = RequestInterceptor.class.getDeclaredField("authMap");
        f.setAccessible(true);
        @SuppressWarnings("unchecked")
        java.util.concurrent.ConcurrentHashMap<String, String> authMap =
                (java.util.concurrent.ConcurrentHashMap<String, String>) f.get(req);
        authMap.put(aliceKey, "alice-auth");

        Util.setUserId("bob");
        Util.setClientId("u:bob");
        String bobKey = RequestInterceptor.tenantHostKey("example.com");
        authMap.put(bobKey, "bob-auth");
        assertFalse(aliceKey.equals(bobKey));
        assertEquals("alice-auth", authMap.get(aliceKey));
        assertEquals("bob-auth", authMap.get(bobKey));

        Method clearTenant = SpiderBridge.class.getDeclaredMethod("clearTenant", String.class);
        clearTenant.setAccessible(true);
        clearTenant.invoke(null, "alice");
        assertTrue(authMap.get(aliceKey) == null);
        assertEquals("bob-auth", authMap.get(bobKey));
        clearTenant.invoke(null, "bob");
        assertTrue(authMap.get(bobKey) == null);
    }

    private static void deleteTree(File f) {
        if (f == null || !f.exists()) return;
        File[] kids = f.listFiles();
        if (kids != null) {
            for (File c : kids) deleteTree(c);
        }
        //noinspection ResultOfMethodCallIgnored
        f.delete();
    }
}
