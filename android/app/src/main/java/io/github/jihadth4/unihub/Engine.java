package io.github.jihadth4.unihub;

import android.content.Context;
import android.content.SharedPreferences;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;

import java.io.File;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.Inet4Address;
import java.net.Inet6Address;
import java.net.InetAddress;
import java.net.URL;
import java.security.SecureRandom;

/** Shared helpers: where the engine listens, its secret key, folders and DNS servers. */
final class Engine {
    static final int PORT = 47821;
    static final String BASE = "http://127.0.0.1:" + PORT + "/";

    private Engine() {}

    /** A secret known only to this app; the engine serves its page and host queue only to it. */
    static synchronized String hostKey(Context c) {
        SharedPreferences p = c.getSharedPreferences("engine", Context.MODE_PRIVATE);
        String k = p.getString("hostKey", null);
        if (k == null || k.length() < 32) {
            byte[] b = new byte[24];
            new SecureRandom().nextBytes(b);
            StringBuilder sb = new StringBuilder();
            for (byte x : b) sb.append(String.format("%02x", x));
            k = sb.toString();
            p.edit().putString("hostKey", k).apply();
        }
        return k;
    }

    static File dataDir(Context c) {
        File d = new File(c.getFilesDir(), "data");
        d.mkdirs();
        return d;
    }

    /** Organized lectures and images live in the app's folder on the phone storage. */
    static File rootBase(Context c) {
        File ext = c.getExternalFilesDir(null);
        return ext != null ? ext : c.getFilesDir();
    }

    static String dnsServers(Context c) {
        StringBuilder sb = new StringBuilder();
        try {
            ConnectivityManager cm = (ConnectivityManager) c.getSystemService(Context.CONNECTIVITY_SERVICE);
            Network n = cm.getActiveNetwork();
            LinkProperties lp = n == null ? null : cm.getLinkProperties(n);
            if (lp != null) {
                for (InetAddress a : lp.getDnsServers()) {
                    if (a instanceof Inet4Address || (a instanceof Inet6Address && !a.isLinkLocalAddress())) {
                        String h = a.getHostAddress();
                        int pct = h.indexOf('%');
                        if (pct > 0) h = h.substring(0, pct);
                        if (sb.length() > 0) sb.append(',');
                        sb.append(h);
                    }
                }
            }
        } catch (Exception ignored) {
        }
        return sb.toString();
    }

    /** True when the engine answers on its port. */
    static boolean alive() {
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL(BASE + "api/ping").openConnection();
            c.setConnectTimeout(800);
            c.setReadTimeout(1500);
            if (c.getResponseCode() != 200) return false;
            try (InputStream in = c.getInputStream()) {
                byte[] b = new byte[256];
                int n = in.read(b);
                return n > 0 && new String(b, 0, n, "UTF-8").contains("unihub");
            }
        } catch (Exception e) {
            return false;
        } finally {
            if (c != null) c.disconnect();
        }
    }
}
