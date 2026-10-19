# Persistent CatVod Python spider runner for KOTV.
import base64
import hashlib
import importlib.util
import json
import os
import re
import socket
import ssl
import struct
import sys
import threading
import time
import traceback


def _prepend_sys_path(*candidates):
    for path in candidates:
        if path and os.path.isdir(path) and path not in sys.path:
            sys.path.insert(0, path)


def _python_home_candidates():
    homes = []
    for key in ("PYTHONHOME", "KOTV_PYTHON_HOME"):
        v = os.environ.get(key, "").strip()
        if v:
            homes.append(v)
    # 进程内 embed：sys.executable 常是 kotv-engine，不能只靠它找 site-packages
    exe = os.path.abspath(getattr(sys, "executable", "") or "")
    if exe:
        exe_dir = os.path.dirname(exe)
        homes.extend([
            exe_dir,
            os.path.dirname(exe_dir),
            os.path.join(os.path.dirname(exe_dir), "python"),
            os.path.join(os.path.dirname(os.path.dirname(exe_dir)), "Resources", "runtime", "python"),
            os.path.join(os.path.dirname(os.path.dirname(exe_dir)), "runtime", "python"),
        ])
    rt = os.environ.get("KOTV_RUNTIME", "").strip()
    if rt:
        homes.append(os.path.join(rt, "python"))
        homes.append(rt)
    # 去重保序
    out, seen = [], set()
    for h in homes:
        h = os.path.abspath(h) if h else ""
        if h and h not in seen:
            seen.add(h)
            out.append(h)
    return out


def _ensure_bundled_site_packages():
    """Windows embed 的 python*._pth 常忽略 PYTHONPATH；显式把 Lib/site-packages 塞进 sys.path。"""
    ver = "python%d.%d" % sys.version_info[:2]
    for home in _python_home_candidates():
        _prepend_sys_path(
            os.path.join(home, "Lib", "site-packages"),
            os.path.join(home, "lib", "site-packages"),
            os.path.join(home, "lib", ver, "site-packages"),
            os.path.join(home, "lib", ver),
            os.path.join(home, "Lib"),
        )
        # 少数布局：python.exe 在 Scripts/ 或 bin/ 下
        parent = os.path.dirname(home)
        _prepend_sys_path(
            os.path.join(parent, "Lib", "site-packages"),
            os.path.join(parent, "lib", "site-packages"),
            os.path.join(parent, "lib", ver, "site-packages"),
        )


_ensure_bundled_site_packages()


def _import_host(name):
    """已在 sys.modules 或能直接导入才算宿主有。失败时不要先塞空包，否则真实导入会被挡住。"""
    if name in sys.modules:
        return sys.modules[name]
    try:
        return importlib.import_module(name)
    except Exception:
        return None


def _object_with(mod, leaf, method):
    if mod is None:
        return None
    inner = getattr(mod, leaf, None)
    if inner is not None and inner is not mod and callable(getattr(inner, method, None)):
        return inner
    if callable(getattr(mod, method, None)):
        return mod
    return None


def _install_proxy_stub(types):
    def ensure_pkg(name):
        if name in sys.modules:
            return sys.modules[name]
        m = types.ModuleType(name)
        m.__path__ = []
        sys.modules[name] = m
        parent, _, leaf = name.rpartition(".")
        if parent:
            setattr(ensure_pkg(parent), leaf, m)
        return m

    ensure_pkg("com.github.catvod")

    class Proxy:
        @staticmethod
        def getPort():
            # 缺省端口与 Go localproxy.DefaultPort 一致
            try:
                return int(os.environ.get("KOTV_PROXY_PORT", "9978") or "9978")
            except Exception:
                return 9978

        @staticmethod
        def getUrl(local=True):
            port = Proxy.getPort()
            if local:
                host = "127.0.0.1"
            else:
                host = (os.environ.get("KOTV_PROXY_HOST") or "").strip() or "127.0.0.1"
            return f"http://{host}:{port}/proxy"

    proxy_mod = types.ModuleType("com.github.catvod.Proxy")
    proxy_mod.Proxy = Proxy
    # `from com.github.catvod import Proxy` 与 `from com.github.catvod.Proxy import getUrl`
    for name in ("getPort", "getUrl"):
        setattr(proxy_mod, name, getattr(Proxy, name))
    sys.modules["com.github.catvod.Proxy"] = proxy_mod
    sys.modules["com.github.catvod"].Proxy = Proxy
    return Proxy


def _install_python_stub(types):
    def ensure_pkg(name):
        if name in sys.modules:
            return sys.modules[name]
        m = types.ModuleType(name)
        m.__path__ = []
        sys.modules[name] = m
        parent, _, leaf = name.rpartition(".")
        if parent:
            setattr(ensure_pkg(parent), leaf, m)
        return m

    ensure_pkg("com.chaquo.python")

    class _CacheDir:
        def getAbsolutePath(self):
            return os.environ.get("KOTV_PY_CACHE") or os.path.dirname(os.path.dirname(__file__))

    class _App:
        def getCacheDir(self):
            return _CacheDir()

    class _Platform:
        def getApplication(self):
            return _App()

    class Python:
        @staticmethod
        def getPlatform():
            return _Platform()

    py_mod = types.ModuleType("com.chaquo.python.Python")
    py_mod.Python = Python
    sys.modules["com.chaquo.python.Python"] = py_mod
    sys.modules["com.chaquo.python"].Python = Python
    return Python


_WS_GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def _ws_send(sock, opcode, data):
    header = bytearray([0x80 | (opcode & 0x0F)])
    n = len(data)
    if n < 126:
        header.append(0x80 | n)
    elif n < 65536:
        header.append(0x80 | 126)
        header += struct.pack("!H", n)
    else:
        header.append(0x80 | 127)
        header += struct.pack("!Q", n)
    mask = os.urandom(4)
    header += mask
    header += bytes(b ^ mask[i % 4] for i, b in enumerate(data))
    sock.sendall(header)


def _ws_recv_exact(sock, n):
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise OSError("closed")
        buf += chunk
    return buf


class _UiLink:
    def __init__(self):
        self._lock = threading.Lock()
        self._send_lock = threading.Lock()
        self._sock = None
        self._replies = {}
        self._started = False
        self._rest = b""
        self._port = 0

    def send(self, text, port):
        data = text.encode("utf-8")
        last = None
        for _ in range(2):
            sock = None
            try:
                with self._lock:
                    self._ensure(port)
                    sock = self._sock
                self._send_frame(sock, 0x1, data)
                return
            except Exception as exc:
                last = exc
                with self._lock:
                    if self._sock is sock:
                        self._drop()
        if last is not None:
            raise last

    def _send_frame(self, sock, opcode, data):
        with self._send_lock:
            _ws_send(sock, opcode, data)

    def take(self, sid):
        with self._lock:
            if self._sock is None and self._port:
                try:
                    self._ensure(self._port)
                except Exception:
                    pass
            q = self._replies.get(sid)
            if not q:
                return ""
            return q.pop(0)

    def _ensure(self, port):
        if port:
            self._port = int(port)
        if self._sock is not None:
            return
        port = int(self._port or 0) or 9978
        key = base64.b64encode(os.urandom(16)).decode("ascii")
        raw = socket.create_connection(("127.0.0.1", port), 5)
        raw.sendall((
            "GET /ui?role=src HTTP/1.1\r\n"
            "Host: 127.0.0.1:%d\r\n"
            "Upgrade: websocket\r\n"
            "Connection: Upgrade\r\n"
            "Sec-WebSocket-Key: %s\r\n"
            "Sec-WebSocket-Version: 13\r\n\r\n" % (port, key)
        ).encode("ascii"))
        buf = b""
        while b"\r\n\r\n" not in buf:
            chunk = raw.recv(4096)
            if not chunk:
                raw.close()
                raise OSError("ui handshake")
            buf += chunk
            if len(buf) > 8192:
                raw.close()
                raise OSError("ui handshake")
        head, _, rest = buf.partition(b"\r\n\r\n")
        status = head.split(b"\r\n", 1)[0]
        if b" 101 " not in status:
            raw.close()
            raise OSError("ui handshake")
        accept = base64.b64encode(hashlib.sha1((key + _WS_GUID).encode("ascii")).digest())
        if accept not in head:
            raw.close()
            raise OSError("ui handshake")
        self._sock = raw
        self._rest = rest
        if not self._started:
            self._started = True
            threading.Thread(target=self._read, daemon=True).start()

    def _drop(self):
        sock = self._sock
        self._sock = None
        if sock is not None:
            try:
                sock.close()
            except Exception:
                pass

    def _read(self):
        pending = b""
        while True:
            with self._lock:
                sock = self._sock
                extra = getattr(self, "_rest", b"")
                self._rest = b""
            if sock is None:
                time.sleep(0.05)
                continue
            try:
                if extra:
                    pending += extra
                while True:
                    if len(pending) < 2:
                        pending += _ws_recv_exact(sock, 2 - len(pending))
                    b0, b1 = pending[0], pending[1]
                    need = 2
                    n = b1 & 0x7F
                    masked = (b1 & 0x80) != 0
                    if n == 126:
                        need = 4
                    elif n == 127:
                        need = 10
                    if len(pending) < need:
                        pending += _ws_recv_exact(sock, need - len(pending))
                    if n == 126:
                        n = struct.unpack("!H", pending[2:4])[0]
                        pos = 4
                    elif n == 127:
                        n = struct.unpack("!Q", pending[2:10])[0]
                        pos = 10
                    else:
                        pos = 2
                    if masked:
                        if len(pending) < pos + 4:
                            pending += _ws_recv_exact(sock, pos + 4 - len(pending))
                        mask = pending[pos:pos + 4]
                        pos += 4
                    else:
                        mask = b""
                    if len(pending) < pos + n:
                        pending += _ws_recv_exact(sock, pos + n - len(pending))
                    data = pending[pos:pos + n]
                    pending = pending[pos + n:]
                    if mask:
                        data = bytes(b ^ mask[i % 4] for i, b in enumerate(data))
                    opcode = b0 & 0x0F
                    if opcode == 0x9:
                        self._send_frame(sock, 0xA, data)
                    elif opcode == 0x8:
                        raise OSError("closed")
                    elif opcode == 0x1:
                        self._remember(data.decode("utf-8", "replace"))
            except Exception:
                with self._lock:
                    if self._sock is sock:
                        self._drop()
                pending = b""
                time.sleep(0.05)

    def _remember(self, text):
        text = (text or "").strip()
        if not text.startswith("{"):
            return
        try:
            data = json.loads(text)
        except Exception:
            return
        if not isinstance(data, dict):
            return
        sid = str(data.get("id") or "").strip()
        if not sid:
            return
        with self._lock:
            self._replies.setdefault(sid, []).append(text)


_UI = _UiLink()


def _install_java_module_stubs():
    """桌面 CPython 没有这些类时才装桩。解释器里已经能导入的保持不动。"""
    import types

    proxy_cls = _object_with(_import_host("com.github.catvod.Proxy"), "Proxy", "getPort")
    if proxy_cls is None:
        proxy_cls = _object_with(_import_host("com.github.catvod"), "Proxy", "getPort")
    if proxy_cls is None:
        proxy_cls = _install_proxy_stub(types)

    if _object_with(_import_host("com.chaquo.python.Python"), "Python", "getPlatform") is None:
        if _object_with(_import_host("com.chaquo.python"), "Python", "getPlatform") is None:
            _install_python_stub(types)

    # 安卓解释器自带 java。这里再写 sys.modules["java"] 会盖掉它。
    if not _host_java_present():
        _install_desktop_java(types, proxy_cls)


def _host_java_present():
    mod = sys.modules.get("java")
    if mod is None:
        try:
            import java as mod
        except Exception:
            return False
    probe = getattr(mod, "jclass", None)
    if not callable(probe):
        return False
    try:
        probe("java.lang.Object")
    except LookupError:
        return False
    except Exception:
        return True
    return True


_MOD_STATIC = 0x0008


def _install_desktop_java(types, proxy_cls):
    """桌面 CPython 没有解释器内的 java。补上脚本打开目录时用到的那几个符号，窗口走本机界面套接字。"""
    import json
    import threading
    import urllib.parse
    import urllib.request

    class _Reflect:
        def __init__(self, name):
            self._name = name

        def getDeclaredMethods(self):
            if self._name == "com.fongmi.android.tv.App":
                return [_JMethod()]
            return []

        def getDeclaredFields(self):
            return []

        def isAssignableFrom(self, other):
            return getattr(other, "_name", "") == "android.app.Activity" and self._name == "android.app.Activity"

    class _JMethod:
        def getModifiers(self):
            return _MOD_STATIC

        def getParameterTypes(self):
            return []

        def getReturnType(self):
            return _Reflect("android.app.Activity")

        def setAccessible(self, flag):
            return None

        def invoke(self, obj, args=None):
            return _Activity()

    class _JavaClass:
        def __init__(self, name):
            self._name = name

        def getClass(self):
            return _Reflect(self._name)

    class _ModifierClass:
        def isStatic(self, modifiers):
            try:
                return bool(int(modifiers) & _MOD_STATIC)
            except Exception:
                return False

    class _ToastClass:
        LENGTH_LONG = 1
        LENGTH_SHORT = 0

        def makeText(self, activity, text, duration):
            return _Toast("" if text is None else str(text))

    class _BuilderClass:
        def __call__(self, *args):
            return _DialogBuilder()

    class _Toast:
        def __init__(self, text):
            self.text = text

        def show(self):
            _post_msg(self.text)

    class _Activity:
        def runOnUiThread(self, runnable):
            runnable.run()

    class _Dialog:
        def __init__(self, builder):
            self.builder = builder
            self.sid = "py-dir-%d" % time.time_ns()
            self._stop = threading.Event()
            self._thread = None

        def dismiss(self):
            self._stop.set()
            _post_msg("UI_CLOSE:" + json.dumps({"id": self.sid}, ensure_ascii=False))

        def open(self):
            labels = list(self.builder.items or [])
            actions = []
            if self.builder.positive:
                actions.append({"id": "positive", "label": self.builder.positive[0], "dismiss": True})
            if self.builder.negative_label:
                actions.append({"id": "negative", "label": self.builder.negative_label, "dismiss": True})
            doc = {
                "id": self.sid,
                "title": self.builder.title or "",
                "timeoutMs": 0,
                "elements": [
                    {"type": "button", "id": "item:%d" % i, "text": str(label), "dismiss": False}
                    for i, label in enumerate(labels)
                ],
                "actions": actions,
            }
            _post_msg("UI:" + json.dumps(doc, ensure_ascii=False))
            self._thread = threading.Thread(target=self._wait, daemon=True)
            self._thread.start()
            return self

        def _wait(self):
            while not self._stop.is_set():
                raw = _take_reply(self.sid)
                if not raw:
                    time.sleep(0.05)
                    continue
                action = _reply_action(raw)
                if action in ("", "shown", "closed"):
                    if action == "closed":
                        self._stop.set()
                    continue
                if action in ("dismiss", "timeout", "negative"):
                    self._stop.set()
                    listener = self.builder.negative
                    if action == "negative" and listener is not None:
                        try:
                            listener.onClick(self, -2)
                        except Exception:
                            pass
                    continue
                if action == "positive":
                    self._stop.set()
                    listener = self.builder.positive[1] if self.builder.positive else None
                    if listener is not None:
                        try:
                            listener.onClick(self, -1)
                        except Exception:
                            pass
                    continue
                if action.startswith("item:"):
                    try:
                        idx = int(action.split(":", 1)[1])
                    except Exception:
                        continue
                    listener = self.builder.item_click
                    if listener is not None:
                        try:
                            listener.onClick(self, idx)
                        except Exception:
                            pass

    class _DialogBuilder:
        def __init__(self):
            self.title = ""
            self.items = []
            self.item_click = None
            self.positive = None
            self.negative = None
            self.negative_label = ""

        def setTitle(self, title):
            self.title = "" if title is None else str(title)
            return self

        def setItems(self, arr, listener):
            self.items = list(arr or [])
            self.item_click = listener
            return self

        def setPositiveButton(self, text, listener):
            self.positive = (str(text), listener)
            return self

        def setNegativeButton(self, text, listener):
            self.negative_label = str(text)
            self.negative = listener
            return self

        def show(self):
            return _Dialog(self).open()

    class _JvmObj:
        def __init__(self, handle, is_class=False):
            self._handle = handle
            self._is_class = is_class

        def __call__(self, *args):
            if not self._is_class:
                raise TypeError("not a class")
            return _jvm_decode(_jvm_post({"op": "new", "handle": self._handle, "args": _jvm_args(args)}))

        def __getattr__(self, name):
            if name.startswith("_"):
                raise AttributeError(name)
            got = _jvm_post({"op": "field", "handle": self._handle, "name": name})
            if got.get("miss") == "field":
                def call(*args, _name=name):
                    return _jvm_decode(_jvm_post({
                        "op": "call",
                        "handle": self._handle,
                        "name": _name,
                        "args": _jvm_args(args),
                    }))
                return call
            return _jvm_decode(got)

    def _jvm_args(args):
        out = []
        for item in args:
            if item is None:
                out.append({"t": "z"})
            elif isinstance(item, _JvmObj):
                out.append({"t": "h", "v": item._handle})
            elif isinstance(item, bool):
                out.append({"t": "b", "v": item})
            elif isinstance(item, (int, float)):
                out.append({"t": "n", "v": item})
            else:
                out.append({"t": "s", "v": str(item)})
        return out

    def _jvm_decode(out):
        kind = out.get("t")
        if kind == "h":
            return _JvmObj(int(out.get("v")), is_class=out.get("k") == "class")
        if kind == "b":
            return bool(out.get("v"))
        if kind == "n":
            value = out.get("v")
            if isinstance(value, float) and value.is_integer():
                return int(value)
            return value
        if kind == "s":
            return out.get("v")
        return None

    def _local_open(path, data=None, content_type=None, timeout=5):
        req = urllib.request.Request(_base() + path, data=data)
        if content_type:
            req.add_header("Content-Type", content_type)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        return opener.open(req, timeout=timeout)

    def _jvm_post(payload):
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        with _local_open("/java", data, "application/json; charset=utf-8", 15) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
        out = json.loads(raw or "{}")
        if not out.get("ok"):
            if out.get("miss") == "field":
                return out
            if out.get("miss") == "class":
                raise LookupError(out.get("error") or payload.get("name") or "not found")
            raise RuntimeError(out.get("error") or "java")
        return out

    def _jvm_class(name):
        return _jvm_decode(_jvm_post({"op": "class", "name": name}))

    def jclass(name):
        if name == "com.github.catvod.Proxy":
            return proxy_cls
        # 这几个没有 JVM 类，或者 show() 到不了界面，仍走本机窗口。
        if name == "android.widget.Toast":
            return _ToastClass()
        if name in (
            "com.google.android.material.dialog.MaterialAlertDialogBuilder",
            "android.app.AlertDialog$Builder",
        ):
            return _BuilderClass()
        if name == "java.lang.reflect.Modifier":
            return _ModifierClass()
        if name in (
            "com.fongmi.android.tv.App",
            "android.app.Activity",
            "android.content.DialogInterface$OnClickListener",
            "java.lang.Runnable",
        ):
            return _JavaClass(name)
        return _jvm_class(name)

    def dynamic_proxy(spec):
        class _Proxy:
            pass
        return _Proxy

    def jarray(component):
        def build(seq):
            return list(seq or [])
        return build

    def _base():
        port = proxy_cls.getPort()
        return "http://127.0.0.1:%s" % port

    def _post_msg(msg):
        _UI.send(msg, proxy_cls.getPort())

    def _take_reply(sid):
        return _UI.take(sid)

    def _reply_action(raw):
        raw = (raw or "").strip()
        if not raw:
            return ""
        if raw.startswith("{"):
            try:
                data = json.loads(raw)
            except Exception:
                return ""
            if isinstance(data, dict):
                return str(data.get("action") or "").strip()
        return raw

    java_mod = types.ModuleType("java")
    java_mod.jclass = jclass
    java_mod.dynamic_proxy = dynamic_proxy
    java_mod.jarray = jarray
    lang = types.ModuleType("java.lang")
    lang.__path__ = []

    class String:
        pass

    lang.String = String
    java_mod.lang = lang
    sys.modules["java.lang"] = lang
    sys.modules["java"] = java_mod


_install_java_module_stubs()


def _ensure_ssl_certs():
    """桌面/embed Python 常缺 CA，补齐后让 requests/urlopen 可用 HTTPS。"""
    candidates = []
    try:
        import certifi

        candidates.append(certifi.where())
    except Exception:
        pass

    # Windows embed（PythonVista / PBS）常见 certifi 布局
    exe_dir = os.path.dirname(os.path.abspath(sys.executable))
    for rel in (
        os.path.join(exe_dir, "Lib", "site-packages", "certifi", "cacert.pem"),
        os.path.join(exe_dir, "lib", "site-packages", "certifi", "cacert.pem"),
    ):
        candidates.append(rel)

    candidates.extend([
        os.environ.get("SSL_CERT_FILE", ""),
        os.environ.get("REQUESTS_CA_BUNDLE", ""),
        "/etc/ssl/cert.pem",
        "/etc/ssl/certs/ca-certificates.crt",
        "/etc/pki/tls/certs/ca-bundle.crt",
    ])
    try:
        import glob

        candidates.extend(glob.glob("/opt/homebrew/etc/openssl@*/cert.pem"))
        candidates.extend(glob.glob("/usr/local/etc/openssl@*/cert.pem"))
        candidates.extend(
            glob.glob("/Library/Frameworks/Python.framework/Versions/*/etc/openssl/cert.pem")
        )
    except Exception:
        pass

    cafile = next((p for p in candidates if p and os.path.isfile(p)), None)
    if cafile:
        os.environ.setdefault("SSL_CERT_FILE", cafile)
        os.environ.setdefault("REQUESTS_CA_BUNDLE", cafile)
        os.environ.setdefault("CURL_CA_BUNDLE", cafile)
        try:
            ssl._create_default_https_context = lambda: ssl.create_default_context(cafile=cafile)
        except Exception:
            pass
        return

    # 仍找不到证书时降级，避免部分源整站不可用
    try:
        ssl._create_default_https_context = ssl._create_unverified_context
    except Exception:
        pass


_ensure_ssl_certs()

from urllib.parse import urljoin
from urllib.request import urlopen
import base64

script = os.path.abspath(sys.argv[1])
site_key = sys.argv[2]
ext = sys.argv[3]
api = sys.argv[4]
cache = os.path.abspath(sys.argv[5])

_prepend_sys_path(cache, os.path.dirname(script), os.path.join(cache, "base"))

# 标准库 / 捆绑三方：禁止当依赖从爬虫源目录下载，否则会把 cache 里的假文件盖住真模块。
_STDLIB_TOP = set(getattr(sys, "stdlib_module_names", ())) | {
    "abc", "argparse", "array", "asyncio", "atexit", "base64", "binascii", "bisect",
    "builtins", "bz2", "calendar", "cmath", "codecs", "collections", "concurrent",
    "contextlib", "copy", "csv", "ctypes", "dataclasses", "datetime", "decimal",
    "dis", "email", "encodings", "enum", "errno", "fnmatch", "fractions", "functools",
    "gc", "getopt", "getpass", "gettext", "glob", "gzip", "hashlib", "heapq", "hmac",
    "html", "http", "idlelib", "imaplib", "importlib", "inspect", "io", "ipaddress",
    "itertools", "json", "keyword", "linecache", "locale", "logging", "lzma", "math",
    "mimetypes", "mmap", "multiprocessing", "netrc", "numbers", "operator", "os",
    "pathlib", "pickle", "pkgutil", "platform", "plistlib", "poplib", "posixpath",
    "pprint", "profile", "pstats", "pty", "pwd", "py_compile", "queue", "quopri",
    "random", "re", "reprlib", "secrets", "select", "selectors", "shelve", "shlex",
    "shutil", "signal", "site", "smtplib", "socket", "socketserver", "sqlite3", "ssl",
    "stat", "statistics", "string", "struct", "subprocess", "sys", "sysconfig",
    "tarfile", "tempfile", "textwrap", "threading", "time", "timeit", "token",
    "tokenize", "tomllib", "trace", "traceback", "tracemalloc", "types", "typing",
    "unicodedata", "unittest", "urllib", "uuid", "venv", "warnings", "wave",
    "weakref", "webbrowser", "xml", "xmlrpc", "zipfile", "zipimport", "zlib",
    "_thread", "__future__",
    # 常见捆绑库（不应从爬虫仓下载）
    "requests", "urllib3", "certifi", "charset_normalizer", "idna", "lxml", "bs4",
    "beautifulsoup4", "Crypto", "Cryptodome", "PIL", "numpy", "socks", "websocket",
}


def _is_blocked_dep_name(name):
    base = os.path.basename(str(name)).replace(".py", "").strip()
    if not base:
        return True
    top = base.split(".")[0]
    return top in _STDLIB_TOP


def _local_package_dir(name):
    """cache/<name>/ 已是包（如 Chaquopy 的 base.spider）时返回目录。"""
    top = os.path.basename(str(name)).replace(".py", "").strip().split(".")[0]
    if not top:
        return ""
    pkg = os.path.join(cache, top)
    if not os.path.isdir(pkg):
        return ""
    if os.path.isfile(os.path.join(pkg, "__init__.py")) or os.path.isfile(os.path.join(pkg, "spider.py")):
        return pkg
    return ""


def _looks_like_python_source(data):
    """拒绝把接口错误 JSON 写进 cache（否则会盖住 stdlib，如 concurrent.py）。"""
    if not data:
        return False
    head = data.lstrip()[:800]
    if not head:
        return False
    if head.startswith(b"{") or head.startswith(b"["):
        # 爬虫仓 404/500 常返回 {"code":500,"message":"...","data":null}
        if b'"code"' in head[:400] or b'"message"' in head[:400] or b'"data"' in head[:400]:
            return False
        # 纯 JSON 依赖极少见；宁可跳过也不要毒化 cache
        return False
    return True


def _scrub_poisoned_cache():
    """清掉已写入的假依赖（历史 concurrent.py 等）。"""
    try:
        names = os.listdir(cache)
    except Exception:
        return
    for fn in names:
        if not fn.endswith(".py"):
            continue
        path = os.path.join(cache, fn)
        try:
            with open(path, "rb") as f:
                data = f.read(800)
            # stdlib 同名文件一律删（绝不该在 cache）；其它文件若是错误 JSON 也删
            top = fn[:-3].split(".")[0]
            if top in _STDLIB_TOP or not _looks_like_python_source(data):
                os.remove(path)
                print("[pyrunner] removed poisoned cache file: %s" % fn, file=sys.stderr)
        except Exception:
            pass


_scrub_poisoned_cache()

# 若历史误下了 cache/base.py，会盖住 cache/base/ 包（from base.spider）。
for _shadow in list(os.listdir(cache)) if os.path.isdir(cache) else []:
    if not _shadow.endswith(".py"):
        continue
    _top = _shadow[:-3]
    if _local_package_dir(_top):
        try:
            os.remove(os.path.join(cache, _shadow))
            print("[pyrunner] removed package-shadowing file: %s" % _shadow, file=sys.stderr)
        except Exception:
            pass


def _download_dep(name):
    """从爬虫 api 同目录拉取依赖 py 到 cache（失败忽略，由后续 import 报错）。

    依赖来自 getDependence()；每次 init 覆盖写入。
    内置 base.spider 包不从仓拉 base.py。
    """
    name = name if str(name).endswith(".py") else str(name) + ".py"
    if _is_blocked_dep_name(name):
        return
    # Chaquopy 自带 base/spider.py；KOTV 写入 cache/base/。勿再拉同名 .py 以免盖包。
    if _local_package_dir(name):
        return
    target = os.path.join(cache, os.path.basename(name))
    if not str(api).startswith("http"):
        return
    # api 常是 …/foo.py，urljoin 会落到同级 …/t4.py
    dep_url = urljoin(api, os.path.basename(name))
    if not str(dep_url).startswith("http"):
        return
    lock = _acquire_download(target + ".lock")
    if lock is None:
        return
    try:
        ctx = ssl._create_unverified_context()
        with urlopen(dep_url, timeout=30, context=ctx) as response:
            data = response.read()
        if not _looks_like_python_source(data):
            print("[pyrunner] skip non-python dep %s from %s" % (name, dep_url), file=sys.stderr)
            return
        tmp = target + ".tmp"
        with open(tmp, "wb") as output:
            output.write(data)
        os.replace(tmp, target)
    except Exception as exc:
        print("[pyrunner] download %s failed: %s" % (name, exc), file=sys.stderr)
    finally:
        _release_download(lock, target + ".lock")


def _acquire_download(lock_path):
    """拿到锁则由本进程下载。别人正在下时等到结束并返回 None，不再另下。"""
    deadline = time.time() + 60
    while time.time() < deadline:
        try:
            return os.open(lock_path, os.O_CREAT | os.O_EXCL | os.O_RDWR)
        except FileExistsError:
            time.sleep(0.05)
            if not os.path.exists(lock_path):
                return None
    try:
        os.remove(lock_path)
    except OSError:
        pass
    try:
        return os.open(lock_path, os.O_CREAT | os.O_EXCL | os.O_RDWR)
    except FileExistsError:
        return None


def _release_download(fd, lock_path):
    try:
        os.close(fd)
    except OSError:
        pass
    try:
        os.remove(lock_path)
    except OSError:
        pass


def _preload_imports_from_source():
    """顶层 import t4 等发生在 init/getDependence 之前，需按源码预拉。

    不把 from base.spider 里的 base 当成要下载的 base.py（走内置包）。
    """
    try:
        with open(script, "r", encoding="utf-8", errors="ignore") as f:
            src = f.read()
    except Exception:
        return
    names = set()
    for m in re.finditer(r"(?:^|\n)\s*(?:import|from)\s+([A-Za-z_][\w]*)", src):
        mod = m.group(1)
        if _is_blocked_dep_name(mod) or _local_package_dir(mod):
            continue
        names.add(mod + ".py")
    # 常见伴侣模块优先
    for extra in ("t4.py", "utils.py", "common.py"):
        if extra.replace(".py", "") in src or extra in names:
            names.add(extra)
    for name in sorted(names):
        _download_dep(name)


_preload_imports_from_source()

spec = importlib.util.spec_from_file_location("kotv_spider_" + site_key, script)
mod = importlib.util.module_from_spec(spec)
sp = None
load_error = None
try:
    spec.loader.exec_module(mod)
    sp = mod.Spider() if hasattr(mod, "Spider") else None
    if sp is not None:
        sp.siteKey = site_key
except Exception as exc:
    load_error = exc
    traceback.print_exc(file=sys.stderr)


def download_dependencies():
 # init 时下载依赖并覆盖写入
    if sp is None or not hasattr(sp, "getDependence"):
        return
    for item in sp.getDependence() or []:
        _download_dep(item)


def encode_result(value):
    if value is None:
        return "{}"
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False)
    return str(value)


def encode_proxy_result(value):
    """localProxy 返回 list，body 可为 bytes。
    JSON-IPC 无法传 bytes，转为 base64 字符串并置 flag=1。
    None / 非法 → 空串，由 Go 侧判 invalid proxy response。
    """
    if value is None:
        return ""
    if not isinstance(value, (list, tuple)):
        return encode_result(value)
    out = list(value)
    while len(out) < 3:
        out.append(None)
    body = out[2]
    flag = 0
    if len(out) > 4 and out[4] is not None:
        try:
            flag = int(out[4])
        except Exception:
            flag = 0
    if isinstance(body, memoryview):
        body = body.tobytes()
    if isinstance(body, (bytes, bytearray)):
        out[2] = base64.b64encode(bytes(body)).decode("ascii")
        flag = 1
    elif body is None:
        out[2] = ""
    elif not isinstance(body, (str, int, float, bool, dict, list)):
        out[2] = str(body)
    # headers
    if len(out) < 4:
        out.append({})
    elif out[3] is None:
        out[3] = {}
    elif hasattr(out[3], "items") and not isinstance(out[3], dict):
        try:
            out[3] = {str(k): str(v) for k, v in out[3].items()}
        except Exception:
            out[3] = {}
    while len(out) < 5:
        out.append(None)
    out[4] = flag
    return json.dumps(out, ensure_ascii=False)


def invoke(method, args):
    if load_error is not None:
        raise load_error
    if sp is None and mod is None:
        raise RuntimeError("爬虫模块未加载")
    if sp is None:
        fn = getattr(mod, method, None)
    else:
        fn = getattr(sp, method, None)
        if fn is None and method.endswith("Content"):
            fn = getattr(sp, method.replace("Content", ""), None)
    if fn is None:
        return "{}"
    if method == "init":
        download_dependencies()
        sk = args.get("siteKey")
        if sk and sp is not None:
            sp.siteKey = sk
        return encode_result(fn(args.get("extend", ext)))
    if method == "homeContent":
        return encode_result(fn(args.get("filter", True)))
    if method == "homeVideoContent":
        return encode_result(fn())
    if method == "categoryContent":
        return encode_result(fn(args.get("tid", ""), args.get("pg", "1"), args.get("filter", True), args.get("extend") or {}))
    if method == "detailContent":
        return encode_result(fn(args.get("ids") or []))
    if method == "searchContent":
        pg = args.get("pg")
        if pg is None or str(pg).strip() == "":
            pg = "1"
        return encode_result(fn(args.get("key", ""), args.get("quick", False), pg))
    if method == "playerContent":
        return encode_result(fn(args.get("flag", ""), args.get("id", ""), args.get("vipFlags") or []))
    if method == "liveContent":
        # TV app.py：原样返回，不 json.dumps；None → 空串。
        value = fn(args.get("url", ""))
        if value is None:
            return ""
        if isinstance(value, (dict, list)):
            return json.dumps(value, ensure_ascii=False)
        return str(value)
    if method == "localProxy":
        return encode_proxy_result(fn(args.get("params") or {}))
    if method == "action":
        return encode_result(fn(args.get("action", "")))
    if method == "isVideoFormat":
        return encode_result(fn(args.get("url", "")))
    if method == "manualVideoCheck":
        return encode_result(fn())
    if method == "destroy":
        return encode_result(fn())
    return encode_result(fn())


def kotv_dispatch_json(line):
    """供 CGO 嵌入 CPython 调用：单行 JSON 请求 → JSON 响应字符串。"""
    line = (line or "").strip()
    if not line:
        return json.dumps({"id": None, "ok": False, "error": "empty request"}, ensure_ascii=False)
    req = json.loads(line)
    try:
        result = invoke(req.get("method", ""), req.get("args") or {})
        return json.dumps({"id": req.get("id"), "ok": True, "result": result}, ensure_ascii=False)
    except Exception as exc:
        traceback.print_exc(file=sys.stderr)
        return json.dumps({"id": req.get("id"), "ok": False, "error": str(exc)}, ensure_ascii=False)


def kotv_repl():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        print(kotv_dispatch_json(line), flush=True)


if __name__ == "__main__":
    kotv_repl()
