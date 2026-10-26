package android.content;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.util.Map;
import java.util.Properties;

/**
 * 磁盘落盘的 SharedPreferences，用 XML 持久化 cookie/token 的行为。
 * 存储为 {name}.properties，值带类型前缀（s/i/l/f/b）以还原原始类型。
 */
final class FileSharedPreferences extends MapSharedPreferences {
    private final File file;

    FileSharedPreferences(File dir, String name) {
        this.file = new File(dir, (name == null || name.isEmpty() ? "default" : name) + ".properties");
        load();
    }

    public Editor edit() { return newEditor(); }

    protected void afterApply() { persist(); }

    private void load() {
        if (!file.exists()) return;
        Properties props = new Properties();
        try (FileInputStream in = new FileInputStream(file)) {
            props.load(in);
        } catch (Exception ignored) {
            return;
        }
        for (String key : props.stringPropertyNames()) {
            values.put(key, decode(props.getProperty(key)));
        }
    }

    private void persist() {
        Properties props = new Properties();
        synchronized (values) {
            for (Map.Entry<String, Object> entry : values.entrySet()) {
                props.setProperty(entry.getKey(), encode(entry.getValue()));
            }
        }
        try {
            File parent = file.getParentFile();
            if (parent != null) parent.mkdirs();
            try (FileOutputStream out = new FileOutputStream(file)) {
                props.store(out, "KOTV bridge prefs");
            }
        } catch (Exception ignored) {
        }
    }

    private static String encode(Object value) {
        if (value instanceof String) return "s:" + value;
        if (value instanceof Integer) return "i:" + value;
        if (value instanceof Long) return "l:" + value;
        if (value instanceof Float) return "f:" + value;
        if (value instanceof Boolean) return "b:" + value;
        return "s:" + String.valueOf(value);
    }

    private static Object decode(String raw) {
        if (raw == null || raw.length() < 2 || raw.charAt(1) != ':') return raw;
        String body = raw.substring(2);
        try {
            switch (raw.charAt(0)) {
                case 'i': return Integer.valueOf(body);
                case 'l': return Long.valueOf(body);
                case 'f': return Float.valueOf(body);
                case 'b': return Boolean.valueOf(body);
                default: return body;
            }
        } catch (Exception e) {
            return body;
        }
    }
}
