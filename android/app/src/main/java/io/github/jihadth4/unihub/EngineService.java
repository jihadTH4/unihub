package io.github.jihadth4.unihub;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.util.Log;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.Map;

/**
 * Runs the UniHub engine (the same Go program as the Windows version, shipped as
 * libunihub.so) as a child process and keeps it alive, so Moodle sync, the search index
 * and notifications work even when the app's screen is closed. It also carries out what
 * the engine asks the phone to do (open a file, notify, install an update).
 */
public class EngineService extends Service {
    static final String TAG = "UniHub";
    static final String ACTION_STOP = "io.github.jihadth4.unihub.STOP";
    static final String CH_ENGINE = "engine";
    static final String CH_GENERAL = "general";
    static final String CH_UPDATE = "update";
    static final int ID_ONGOING = 1;
    static final int ID_UPDATE = 2;

    private final Handler main = new Handler(Looper.getMainLooper());
    private volatile Process proc;
    private volatile boolean stopping;
    private volatile boolean quitRequested;
    private Thread poller;
    private int notifId = 100;
    private long[] restarts = new long[0];

    static void start(Context c) {
        Intent i = new Intent(c, EngineService.class);
        try {
            if (Build.VERSION.SDK_INT >= 26) c.startForegroundService(i);
            else c.startService(i);
        } catch (Exception e) {
            Log.w(TAG, "cannot start engine service", e);
        }
    }

    @Override
    public void onCreate() {
        super.onCreate();
        createChannels();
        Notification n = ongoing();
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(ID_ONGOING, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        } else {
            startForeground(ID_ONGOING, n);
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            shutdown();
            return START_NOT_STICKY;
        }
        stopping = false;
        quitRequested = false;
        new Thread(this::ensureEngine, "engine-start").start();
        return START_STICKY;
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public void onDestroy() {
        stopping = true;
        Process p = proc;
        if (p != null) p.destroy();
        super.onDestroy();
    }

    // ---------------------------------------------------------------- engine process

    private synchronized void ensureEngine() {
        if (stopping) return;
        Process p = proc;
        if ((p == null || !p.isAlive()) && !Engine.alive()) {
            launch();
        }
        if (poller == null || !poller.isAlive()) {
            poller = new Thread(this::pollLoop, "engine-host");
            poller.start();
        }
    }

    private void launch() {
        File exe = new File(getApplicationInfo().nativeLibraryDir, "libunihub.so");
        ProcessBuilder pb = new ProcessBuilder(exe.getAbsolutePath(), "--background", "--wait-port");
        Map<String, String> env = pb.environment();
        env.put("UNIHUB_DATA", Engine.dataDir(this).getAbsolutePath());
        env.put("UNIHUB_HOST_KEY", Engine.hostKey(this));
        env.put("UNIHUB_ROOT", Engine.rootBase(this).getAbsolutePath());
        env.put("UNIHUB_DNS", Engine.dnsServers(this));
        env.put("HOME", getFilesDir().getAbsolutePath());
        env.put("TMPDIR", getCacheDir().getAbsolutePath());
        pb.directory(getFilesDir());
        pb.redirectErrorStream(true);
        pb.redirectOutput(new File(getFilesDir(), "engine.log"));
        try {
            final Process started = pb.start();
            proc = started;
            Log.i(TAG, "engine started");
            Thread w = new Thread(() -> watch(started), "engine-watch");
            w.setDaemon(true);
            w.start();
        } catch (Exception e) {
            Log.e(TAG, "cannot start engine", e);
            toast("تعذر تشغيل محرك البرنامج: " + e.getMessage());
        }
    }

    private void watch(Process p) {
        int code;
        try {
            code = p.waitFor();
        } catch (InterruptedException e) {
            return;
        }
        Log.i(TAG, "engine exited with " + code);
        if (proc == p) proc = null;
        if (stopping) return;
        if (quitRequested) {
            main.post(this::shutdown);
            return;
        }
        // crashed or replaced: start again, but not in a tight loop
        long now = System.currentTimeMillis();
        int recent = 0;
        for (long t : restarts) if (now - t < 10 * 60_000) recent++;
        if (recent >= 5) {
            Log.e(TAG, "engine keeps stopping, giving up");
            toast("توقف محرك البرنامج بشكل متكرر. افتح التطبيق مرة أخرى لاحقاً.");
            main.post(this::shutdown);
            return;
        }
        long[] r = new long[recent + 1];
        int j = 0;
        for (long t : restarts) if (now - t < 10 * 60_000) r[j++] = t;
        r[j] = now;
        restarts = r;
        main.postDelayed(() -> new Thread(this::ensureEngine, "engine-restart").start(), 2000);
    }

    /** Stops the engine and this service, and closes the app's screen (the "quit" button). */
    private void shutdown() {
        stopping = true;
        Process p = proc;
        if (p != null) p.destroy();
        MainActivity a = MainActivity.current();
        if (a != null) a.finishAndRemoveTask();
        stopForeground(true);
        stopSelf();
    }

    // ---------------------------------------------------------------- host commands

    private void pollLoop() {
        String key = Engine.hostKey(this);
        while (!stopping) {
            HttpURLConnection c = null;
            try {
                c = (HttpURLConnection) new URL(Engine.BASE + "api/host/poll").openConnection();
                c.setConnectTimeout(2000);
                c.setReadTimeout(40_000);
                c.setRequestProperty("X-UniHub-Host", key);
                if (c.getResponseCode() != 200) {
                    Thread.sleep(2000);
                    continue;
                }
                JSONArray cmds = new JSONArray(read(c.getInputStream()));
                for (int i = 0; i < cmds.length(); i++) {
                    final JSONObject cmd = cmds.getJSONObject(i);
                    main.post(() -> handle(cmd));
                }
            } catch (Exception e) {
                try {
                    Thread.sleep(2000);
                } catch (InterruptedException ie) {
                    return;
                }
            } finally {
                if (c != null) c.disconnect();
            }
        }
    }

    private static String read(InputStream in) throws Exception {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        byte[] b = new byte[8192];
        int n;
        while ((n = in.read(b)) > 0) out.write(b, 0, n);
        in.close();
        return out.toString("UTF-8");
    }

    private void handle(JSONObject cmd) {
        String op = cmd.optString("op");
        switch (op) {
            case "notify":
                notifyUser(cmd.optString("title"), cmd.optString("body"));
                break;
            case "open":
                openFile(new File(cmd.optString("path")));
                break;
            case "url":
                openUrl(cmd.optString("url"));
                break;
            case "install":
                install(new File(cmd.optString("path")), cmd.optString("version"));
                break;
            case "quit":
                quitRequested = true;
                break;
            default:
                Log.w(TAG, "unknown host command " + op);
        }
    }

    private void openFile(File f) {
        Uri u = FilesProvider.uriFor(this, f);
        if (u == null || !f.isFile()) {
            toast("الملف غير موجود");
            return;
        }
        Intent i = new Intent(Intent.ACTION_VIEW).setDataAndType(u, FilesProvider.mimeOf(f.getName()))
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            startActivity(Intent.createChooser(i, f.getName()).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
        } catch (ActivityNotFoundException e) {
            toast("لا يوجد تطبيق على هاتفك يفتح هذا النوع من الملفات");
        }
    }

    private void openUrl(String url) {
        if (!(url.startsWith("https://") || url.startsWith("http://"))) return;
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
        } catch (ActivityNotFoundException e) {
            toast("لا يوجد متصفح لفتح الرابط");
        }
    }

    private void install(File apk, String version) {
        if (!apk.isFile()) return;
        MainActivity a = MainActivity.current();
        if (a != null && a.isVisible()) {
            a.requestInstall(apk);
            return;
        }
        // the app is in the background: let the user install from a notification
        Intent open = new Intent(this, MainActivity.class).putExtra(MainActivity.EXTRA_INSTALL, apk.getAbsolutePath())
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        PendingIntent pi = PendingIntent.getActivity(this, 2, open, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        Notification n = new Notification.Builder(this, CH_UPDATE)
                .setSmallIcon(R.drawable.ic_stat)
                .setContentTitle("تحديث جديد جاهز" + (version.isEmpty() ? "" : " (" + version + ")"))
                .setContentText("اضغط لتثبيت الإصدار الجديد من منظّم الجامعة")
                .setContentIntent(pi)
                .setAutoCancel(true)
                .build();
        nm().notify(ID_UPDATE, n);
    }

    // ---------------------------------------------------------------- notifications

    private NotificationManager nm() {
        return (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
    }

    private void createChannels() {
        NotificationManager m = nm();
        NotificationChannel e = new NotificationChannel(CH_ENGINE, "يعمل في الخلفية", NotificationManager.IMPORTANCE_MIN);
        e.setDescription("إشعار ثابت يبقي المزامنة مع المودل تعمل والتطبيق مغلق");
        e.setShowBadge(false);
        m.createNotificationChannel(e);
        NotificationChannel g = new NotificationChannel(CH_GENERAL, "المحاضرات والواجبات", NotificationManager.IMPORTANCE_DEFAULT);
        g.setDescription("ملفات جديدة، إشعارات المودل، المواعيد القريبة والملاحظات");
        m.createNotificationChannel(g);
        NotificationChannel u = new NotificationChannel(CH_UPDATE, "التحديثات", NotificationManager.IMPORTANCE_DEFAULT);
        m.createNotificationChannel(u);
    }

    private PendingIntent openApp() {
        Intent i = new Intent(this, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        return PendingIntent.getActivity(this, 1, i, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private Notification ongoing() {
        Intent stop = new Intent(this, EngineService.class).setAction(ACTION_STOP);
        PendingIntent stopPi = PendingIntent.getService(this, 3, stop, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        return new Notification.Builder(this, CH_ENGINE)
                .setSmallIcon(R.drawable.ic_stat)
                .setContentTitle("منظّم الجامعة يعمل")
                .setContentText("المزامنة مع المودل والتنبيهات تعمل في الخلفية")
                .setContentIntent(openApp())
                .addAction(new Notification.Action.Builder(null, "إيقاف", stopPi).build())
                .setOngoing(true)
                .setShowWhen(false)
                .build();
    }

    private void notifyUser(String title, String body) {
        Notification n = new Notification.Builder(this, CH_GENERAL)
                .setSmallIcon(R.drawable.ic_stat)
                .setContentTitle(title)
                .setContentText(body)
                .setStyle(new Notification.BigTextStyle().bigText(body))
                .setContentIntent(openApp())
                .setAutoCancel(true)
                .build();
        nm().notify(notifId++, n);
    }

    private void toast(String msg) {
        main.post(() -> Toast.makeText(getApplicationContext(), msg, Toast.LENGTH_LONG).show());
    }
}
