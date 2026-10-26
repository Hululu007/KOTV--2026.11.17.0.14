package com.github.catvod.bean;

import com.google.gson.Gson;
import com.google.gson.JsonElement;
import com.google.gson.reflect.TypeToken;

import java.lang.reflect.Type;
import java.util.Collections;
import java.util.List;

final class JsonLists {
    private JsonLists() {}

    static <T> List<T> arrayFrom(JsonElement element, Class<T> type) {
        try {
            Type listType = TypeToken.getParameterized(List.class, type).getType();
            List<T> items = new Gson().fromJson(element, listType);
            return items == null ? Collections.emptyList() : items;
        } catch (Exception e) {
            return Collections.emptyList();
        }
    }
}
