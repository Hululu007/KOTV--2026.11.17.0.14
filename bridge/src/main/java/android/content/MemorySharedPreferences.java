package android.content;

final class MemorySharedPreferences extends MapSharedPreferences {
    public Editor edit() { return newEditor(); }

    protected void afterApply() {}
}
