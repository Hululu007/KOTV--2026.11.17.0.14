package com.bobo.kotv

import android.os.Handler
import io.flutter.plugin.common.EventChannel

internal fun emitOnMain(main: Handler, sink: EventChannel.EventSink?, payload: Map<String, Any?>) {
  main.post {
    try {
      sink?.success(payload)
    } catch (_: Throwable) {
    }
  }
}
