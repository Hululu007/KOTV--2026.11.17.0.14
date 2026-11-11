package com.fongmi.android.tv.player.iso;

import java.nio.ByteBuffer;

/**
 * Stub for Media3 libplayer {@code register_iso_protocol} JNI.
 *
 * Media3 {@code libplayer.so} 可能链到这些 native 符号；KOTV <b>不</b>走这套自定义 iso://。
 * DVD/Blu-ray/ISO 由 libmpv（dvdnav + libbluray）完成：Dart 把路径改写成
 * {@code dvd://}/{@code bd://} 并设 dvd-device / bluray-device。
 */
public final class IsoSessionManager {
    private IsoSessionManager() {}

    public static long length(long handle) {
        return 0L;
    }

    public static int readAt(long handle, long position, ByteBuffer buffer, int size) {
        return -1;
    }

    public static void close(long handle) {}

    public static void prepareTrackMetadata(long handle, int title) {}
}
