package android.content;

import android.content.res.AssetManager;

import com.github.catvod.utils.Util;

import java.io.File;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Desktop stub of Android Context for CatVod spiders.
 *
 * <p>三目录分离（由 Go 注入 system properties）：
 * <ul>
 *   <li>{@code kotv.cache.dir} → {@link #getCacheDir()}：公共包体根（jar/js/py）
 *   <li>{@code kotv.user.runtime.root} → {@code run/}：按 {@link Util#tenantKey()} 解析
 *       {@code run/&lt;shared|uid&gt;/jvm/{prefs,files}}（单 JVM 多用户）
 *   <li>{@code kotv.user.dir}：旧单机兼容（整机固定用户根；未设 runtime.root 时使用）
 *   <li>{@code kotv.data.dir} → assets
 * </ul>
 */
public class Context {
    public static final int MODE_PRIVATE = 0;
    public static final String CLIPBOARD_SERVICE = "clipboard";
    /** prefs 按「租户 + 名」缓存，避免跨租户串读。 */
    private static final Map<String, SharedPreferences> PREFERENCES = new ConcurrentHashMap<>();

    private final File cacheRoot;
    private final File legacyUserRoot;
    private final File userRuntimeRoot;
    private final File dataRoot;
    private final boolean dynamicTenantUser;

    public Context() {
        this.cacheRoot = resolveCacheRoot();
        this.userRuntimeRoot = resolveUserRuntimeRoot();
        this.legacyUserRoot = resolveLegacyUserRoot(this.cacheRoot);
        this.dynamicTenantUser = this.userRuntimeRoot != null;
        this.dataRoot = resolveDataRoot(this.cacheRoot);
        this.cacheRoot.mkdirs();
        this.dataRoot.mkdirs();
        if (!this.dynamicTenantUser && this.legacyUserRoot != null) {
            this.legacyUserRoot.mkdirs();
        }
    }

    public Context(File root) {
        // 测试/旧调用：单一根既当 cache 也当 user/data。
        this.cacheRoot = root;
        this.legacyUserRoot = root;
        this.userRuntimeRoot = null;
        this.dynamicTenantUser = false;
        this.dataRoot = root;
        this.cacheRoot.mkdirs();
    }

    /** 公共缓存根：Path.jar / Path.js / Path.py 挂在此下。 */
    public File getCacheDir() {
        cacheRoot.mkdirs();
        return cacheRoot;
    }

    /** 当前租户可写 files（不与公共 jar/js/py 混放）。 */
    public File getFilesDir() {
        return userDirectory("files");
    }

    public AssetManager getAssets() {
        File assets = new File(dataRoot, "assets");
        assets.mkdirs();
        return new AssetManager(assets);
    }

    public SharedPreferences getSharedPreferences(String name, int mode) {
        String prefName = name == null ? "" : name;
        String cacheKey = Util.tenantKey() + "\0" + prefName;
        return PREFERENCES.computeIfAbsent(cacheKey,
                key -> new FileSharedPreferences(userDirectory("prefs"), prefName));
    }

    /** KillUser / clearTenant：丢掉该租户 Prefers 内存缓存（落盘由 Go 擦 UserJVMDir）。 */
    public static void evictTenantPreferences(String tenant) {
        if (tenant == null || tenant.isEmpty()) tenant = "shared";
        String prefix = tenant + "\0";
        PREFERENCES.keySet().removeIf(k -> k != null && k.startsWith(prefix));
    }

    /** clearAll / HardRestart：丢掉全部 Prefers 内存缓存。 */
    public static void evictAllPreferences() {
        PREFERENCES.clear();
    }

    public Context getApplicationContext() {
        return this;
    }

    public ClassLoader getClassLoader() {
        ClassLoader cl = Context.class.getClassLoader();
        return cl != null ? cl : ClassLoader.getSystemClassLoader();
    }

    public Object getSystemService(String name) {
        if (CLIPBOARD_SERVICE.equals(name)) return new ClipboardManager();
        return null;
    }

    public String getPackageName() {
        return "com.bobo.kotv";
    }

    private File userDirectory(String name) {
        File root = currentUserRoot();
        File directory = new File(root, name);
        directory.mkdirs();
        return directory;
    }

    private File currentUserRoot() {
        if (dynamicTenantUser) {
            File tenantJvm = new File(new File(userRuntimeRoot, Util.tenantKey()), "jvm");
            tenantJvm.mkdirs();
            return tenantJvm;
        }
        if (legacyUserRoot != null) {
            legacyUserRoot.mkdirs();
            return legacyUserRoot;
        }
        cacheRoot.mkdirs();
        return cacheRoot;
    }

    private static File resolveCacheRoot() {
        String value = System.getProperty("kotv.cache.dir");
        if (value == null || value.isEmpty()) value = System.getenv("KOTV_CACHE_DIR");
        if (value == null || value.isEmpty()) value = new File(System.getProperty("user.home"), ".kotv").getPath();
        return new File(value);
    }

    /** 单 JVM 多用户：CacheRoot/run */
    private static File resolveUserRuntimeRoot() {
        String value = System.getProperty("kotv.user.runtime.root");
        if (value == null || value.isEmpty()) value = System.getenv("KOTV_USER_RUNTIME_ROOT");
        if (value == null || value.isEmpty()) return null;
        return new File(value);
    }

    private static File resolveLegacyUserRoot(File cacheRoot) {
        String value = System.getProperty("kotv.user.dir");
        if (value == null || value.isEmpty()) value = System.getenv("KOTV_USER_DIR");
        if (value == null || value.isEmpty()) return cacheRoot;
        return new File(value);
    }

    private static File resolveDataRoot(File cacheRoot) {
        String value = System.getProperty("kotv.data.dir");
        if (value == null || value.isEmpty()) value = System.getenv("KOTV_DATA_DIR");
        if (value == null || value.isEmpty()) return cacheRoot;
        return new File(value);
    }
}
