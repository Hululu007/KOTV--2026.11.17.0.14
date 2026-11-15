package android.content;

import android.content.res.AssetManager;

import java.io.File;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Desktop stub of Android Context for CatVod spiders.
 *
 * <p>三目录分离（由 Go 注入 system properties）：
 * <ul>
 *   <li>{@code kotv.cache.dir} → {@link #getCacheDir()}：公共包体根（jar/js/py，与 Go CacheRoot 一致）
 *   <li>{@code kotv.user.dir} → files / Prefers：每用户可变态
 *   <li>{@code kotv.data.dir} → assets（应用数据根）
 * </ul>
 * 未设 {@code kotv.user.dir} 时回落到 cache 根（单机兼容）。
 */
public class Context {
    public static final int MODE_PRIVATE = 0;
    public static final String CLIPBOARD_SERVICE = "clipboard";
    private static final Map<String, SharedPreferences> PREFERENCES = new ConcurrentHashMap<>();

    private final File cacheRoot;
    private final File userRoot;
    private final File dataRoot;

    public Context() {
        this.cacheRoot = resolveCacheRoot();
        this.userRoot = resolveUserRoot(this.cacheRoot);
        this.dataRoot = resolveDataRoot(this.cacheRoot);
        this.cacheRoot.mkdirs();
        this.userRoot.mkdirs();
        this.dataRoot.mkdirs();
    }

    public Context(File root) {
        // 测试/旧调用：单一根既当 cache 也当 user/data。
        this.cacheRoot = root;
        this.userRoot = root;
        this.dataRoot = root;
        this.cacheRoot.mkdirs();
    }

    /** 公共缓存根：Path.jar / Path.js / Path.py 挂在此下。 */
    public File getCacheDir() {
        cacheRoot.mkdirs();
        return cacheRoot;
    }

    /** 每用户可写 files（不与公共 jar/js/py 混放）。 */
    public File getFilesDir() {
        return userDirectory("files");
    }

    public AssetManager getAssets() {
        File assets = new File(dataRoot, "assets");
        assets.mkdirs();
        return new AssetManager(assets);
    }

    public SharedPreferences getSharedPreferences(String name, int mode) {
        return PREFERENCES.computeIfAbsent(name == null ? "" : name,
                key -> new FileSharedPreferences(userDirectory("prefs"), key));
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
        File directory = new File(userRoot, name);
        directory.mkdirs();
        return directory;
    }

    private static File resolveCacheRoot() {
        String value = System.getProperty("kotv.cache.dir");
        if (value == null || value.isEmpty()) value = System.getenv("KOTV_CACHE_DIR");
        if (value == null || value.isEmpty()) value = new File(System.getProperty("user.home"), ".kotv").getPath();
        return new File(value);
    }

    private static File resolveUserRoot(File cacheRoot) {
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
