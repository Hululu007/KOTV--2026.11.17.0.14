package com.github.catvod.bean;

import android.text.TextUtils;

import com.google.gson.JsonElement;
import com.google.gson.annotations.SerializedName;

import java.util.List;

public class Header {

    @SerializedName("host")
    private String host;
    @SerializedName("header")
    private JsonElement header;

    public static List<Header> arrayFrom(JsonElement element) {
        return JsonLists.arrayFrom(element, Header.class);
    }

    public String getHost() {
        return TextUtils.isEmpty(host) ? "" : host;
    }

    public JsonElement getHeader() {
        return header;
    }
}
