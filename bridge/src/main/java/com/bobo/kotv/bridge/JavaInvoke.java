package com.bobo.kotv.bridge;

import com.google.gson.JsonArray;
import com.google.gson.JsonObject;

import java.lang.reflect.Field;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

/** 桌面 Python 按 classpath 调 JVM。对象留句柄，字符串和数字直接返回。 */
public final class JavaInvoke {
    public interface Loaders {
        List<ClassLoader> get();
    }

    private static volatile Loaders more = List::of;
    private static final ConcurrentHashMap<Integer, Object> objects = new ConcurrentHashMap<>();
    private static final AtomicInteger seq = new AtomicInteger(1);

    private JavaInvoke() {}

    public static void setLoaders(Loaders loaders) {
        more = loaders == null ? List::of : loaders;
    }

    public static void reset() {
        objects.clear();
    }

    public static String handle(JsonObject args) {
        try {
            String op = text(args, "op");
            return switch (op) {
                case "class" -> box(load(text(args, "name"))).toString();
                case "new" -> box(construct(object(args), array(args, "args"))).toString();
                case "call" -> box(call(object(args), text(args, "name"), array(args, "args"))).toString();
                case "field" -> field(object(args), text(args, "name")).toString();
                default -> fail("op", "unknown op");
            };
        } catch (ClassNotFoundException e) {
            return fail("class", e.getMessage() == null ? "not found" : e.getMessage());
        } catch (InvocationTargetException e) {
            Throwable cause = e.getCause() == null ? e : e.getCause();
            return fail("", cause.toString());
        } catch (Throwable e) {
            return fail("", e.toString());
        }
    }

    private static Class<?> load(String name) throws ClassNotFoundException {
        try {
            return Class.forName(name);
        } catch (ClassNotFoundException first) {
            List<ClassLoader> loaders;
            try {
                loaders = more.get();
            } catch (Throwable e) {
                loaders = List.of();
            }
            if (loaders != null) {
                for (ClassLoader loader : loaders) {
                    if (loader == null) continue;
                    try {
                        return Class.forName(name, true, loader);
                    } catch (ClassNotFoundException ignored) {
                    }
                }
            }
            throw first;
        }
    }

    private static Object construct(Object target, JsonArray args) throws Exception {
        if (!(target instanceof Class<?> cls)) {
            throw new IllegalArgumentException("not a class");
        }
        int n = args.size();
        for (var ctor : cls.getConstructors()) {
            if (ctor.getParameterCount() != n) continue;
            try {
                return ctor.newInstance(convert(ctor.getParameterTypes(), args));
            } catch (IllegalArgumentException ignored) {
            }
        }
        throw new NoSuchMethodException(cls.getName() + ".<init>");
    }

    private static Object call(Object target, String name, JsonArray args) throws Exception {
        Class<?> cls = target instanceof Class<?> c ? c : target.getClass();
        Object recv = target instanceof Class<?> ? null : target;
        int n = args.size();
        List<Method> hits = new ArrayList<>();
        for (Method method : cls.getMethods()) {
            if (!method.getName().equals(name) || method.getParameterCount() != n) continue;
            boolean stat = Modifier.isStatic(method.getModifiers());
            if (recv == null && !stat) continue;
            if (recv != null && stat) continue;
            hits.add(method);
        }
        for (Method method : hits) {
            try {
                return method.invoke(recv, convert(method.getParameterTypes(), args));
            } catch (IllegalArgumentException ignored) {
            }
        }
        throw new NoSuchMethodException(cls.getName() + "." + name);
    }

    private static JsonObject field(Object target, String name) throws Exception {
        Class<?> cls = target instanceof Class<?> c ? c : target.getClass();
        Field found = null;
        for (Field field : cls.getFields()) {
            if (field.getName().equals(name)) {
                found = field;
                break;
            }
        }
        if (found == null) {
            JsonObject miss = new JsonObject();
            miss.addProperty("ok", false);
            miss.addProperty("miss", "field");
            return miss;
        }
        Object recv = Modifier.isStatic(found.getModifiers()) || target instanceof Class<?> ? null : target;
        return box(found.get(recv));
    }

    private static Object[] convert(Class<?>[] types, JsonArray args) {
        Object[] out = new Object[types.length];
        for (int i = 0; i < types.length; i++) {
            out[i] = convertOne(types[i], args.get(i).getAsJsonObject());
        }
        return out;
    }

    private static Object convertOne(Class<?> type, JsonObject arg) {
        String kind = text(arg, "t");
        if ("z".equals(kind)) return null;
        if ("h".equals(kind)) return objects.get(arg.get("v").getAsInt());
        if ("b".equals(kind)) return arg.get("v").getAsBoolean();
        if ("s".equals(kind)) {
            String value = arg.get("v").getAsString();
            if (type == char.class || type == Character.class) {
                return value.isEmpty() ? '\0' : value.charAt(0);
            }
            return value;
        }
        if ("n".equals(kind)) {
            if (type == int.class || type == Integer.class) return arg.get("v").getAsInt();
            if (type == long.class || type == Long.class) return arg.get("v").getAsLong();
            if (type == short.class || type == Short.class) return arg.get("v").getAsShort();
            if (type == byte.class || type == Byte.class) return arg.get("v").getAsByte();
            if (type == float.class || type == Float.class) return arg.get("v").getAsFloat();
            if (type == double.class || type == Double.class) return arg.get("v").getAsDouble();
            if (type == boolean.class || type == Boolean.class) return arg.get("v").getAsInt() != 0;
            double value = arg.get("v").getAsDouble();
            if (value == Math.rint(value) && value >= Integer.MIN_VALUE && value <= Integer.MAX_VALUE) {
                return (int) value;
            }
            return value;
        }
        return null;
    }

    private static JsonObject box(Object value) {
        JsonObject out = new JsonObject();
        out.addProperty("ok", true);
        if (value == null) {
            out.addProperty("t", "z");
            return out;
        }
        if (value instanceof Boolean b) {
            out.addProperty("t", "b");
            out.addProperty("v", b);
            return out;
        }
        if (value instanceof Byte || value instanceof Short || value instanceof Integer || value instanceof Long) {
            out.addProperty("t", "n");
            out.addProperty("v", ((Number) value).longValue());
            return out;
        }
        if (value instanceof Float || value instanceof Double) {
            out.addProperty("t", "n");
            out.addProperty("v", ((Number) value).doubleValue());
            return out;
        }
        if (value instanceof CharSequence || value instanceof Character) {
            out.addProperty("t", "s");
            out.addProperty("v", String.valueOf(value));
            return out;
        }
        int id = seq.getAndIncrement();
        objects.put(id, value);
        out.addProperty("t", "h");
        out.addProperty("v", id);
        if (value instanceof Class<?>) out.addProperty("k", "class");
        return out;
    }

    private static String fail(String miss, String error) {
        JsonObject out = new JsonObject();
        out.addProperty("ok", false);
        if (miss != null && !miss.isEmpty()) out.addProperty("miss", miss);
        out.addProperty("error", error == null ? "" : error);
        return out.toString();
    }

    private static Object object(JsonObject args) {
        if (!args.has("handle") || args.get("handle").isJsonNull()) {
            throw new IllegalArgumentException("missing handle");
        }
        Object value = objects.get(args.get("handle").getAsInt());
        if (value == null) throw new IllegalArgumentException("stale handle");
        return value;
    }

    private static JsonArray array(JsonObject args, String key) {
        if (!args.has(key) || args.get(key).isJsonNull() || !args.get(key).isJsonArray()) {
            return new JsonArray();
        }
        return args.getAsJsonArray(key);
    }

    private static String text(JsonObject args, String key) {
        if (args == null || !args.has(key) || args.get(key).isJsonNull()) return "";
        return args.get(key).getAsString();
    }
}
