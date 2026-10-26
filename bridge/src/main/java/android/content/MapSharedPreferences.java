package android.content;

import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

/** 内存表上的 SharedPreferences。落盘由子类在 apply 之后做。 */
abstract class MapSharedPreferences implements SharedPreferences {
    protected final Map<String, Object> values = Collections.synchronizedMap(new HashMap<>());

    public Map<String, ?> getAll() { synchronized (values) { return new HashMap<>(values); } }
    public String getString(String key, String fallback) { return value(key, String.class, fallback); }
    public int getInt(String key, int fallback) { return value(key, Integer.class, fallback); }
    public long getLong(String key, long fallback) { return value(key, Long.class, fallback); }
    public float getFloat(String key, float fallback) { return value(key, Float.class, fallback); }
    public boolean getBoolean(String key, boolean fallback) { return value(key, Boolean.class, fallback); }
    public boolean contains(String key) { return values.containsKey(key); }

    protected abstract void afterApply();

    protected Editor newEditor() { return new MapEditor(); }

    private <T> T value(String key, Class<T> type, T fallback) {
        Object value = values.get(key);
        return type.isInstance(value) ? type.cast(value) : fallback;
    }

    private final class MapEditor implements Editor {
        private final Map<String, Object> updates = new HashMap<>();
        private boolean clear;
        public Editor putString(String key, String value) { return put(key, value); }
        public Editor putInt(String key, int value) { return put(key, value); }
        public Editor putLong(String key, long value) { return put(key, value); }
        public Editor putFloat(String key, float value) { return put(key, value); }
        public Editor putBoolean(String key, boolean value) { return put(key, value); }
        public Editor remove(String key) { return put(key, this); }
        public Editor clear() { clear = true; return this; }
        public boolean commit() { apply(); return true; }
        public void apply() {
            synchronized (values) {
                if (clear) values.clear();
                for (Map.Entry<String, Object> entry : updates.entrySet()) {
                    if (entry.getValue() == this) values.remove(entry.getKey());
                    else values.put(entry.getKey(), entry.getValue());
                }
            }
            afterApply();
        }
        private Editor put(String key, Object value) { updates.put(key, value); return this; }
    }
}
