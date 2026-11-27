package io.github.jihadth4.unihub;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;
import android.view.Gravity;
import android.view.View;
import android.view.Window;
import android.view.WindowInsetsController;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.TextView;
import android.widget.Toast;

import java.io.File;
import java.lang.ref.WeakReference;

/** The app's screen: the same interface as on Windows, shown in a WebView. */
public class MainActivity extends Activity {
    static final String EXTRA_INSTALL = "install";
    private static final int REQ_FILES = 10;
    private static WeakReference<MainActivity> ref = new WeakReference<>(null);

    private WebView web;
    private LinearLayout splash;
    private TextView splashText;
    private Button retry;
    private volatile boolean visible;
    private volatile boolean destroyed;
    private boolean pageLoaded;
    private File pendingInstall;
    private ValueCallback<Uri[]> fileCallback;

    static MainActivity current() {
        return ref.get();
    }

    boolean isVisible() {
        return visible;
    }

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        ref = new WeakReference<>(this);
        buildUi();
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 1);
        }
        EngineService.start(this);
        waitForEngine();
        handleIntent(getIntent());
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        handleIntent(intent);
    }

    private void handleIntent(Intent i) {
        String p = i == null ? null : i.getStringExtra(EXTRA_INSTALL);
        if (p != null) {
            i.removeExtra(EXTRA_INSTALL);
            requestInstall(new File(p));
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        visible = true;
        ref = new WeakReference<>(this);
        if (pendingInstall != null && canInstall()) {
            File f = pendingInstall;
            pendingInstall = null;
            startInstaller(f);
        }
        if (pageLoaded) EngineService.start(this); // the engine may have been stopped from the notification
    }

    @Override
    protected void onPause() {
        visible = false;
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        destroyed = true;
        if (web != null) web.destroy();
        super.onDestroy();
    }

    @Override
    public void onBackPressed() {
        if (web != null && web.canGoBack()) web.goBack();
        else moveTaskToBack(true);
    }

    // ---------------------------------------------------------------- UI

    private int dp(int v) {
        return Math.round(v * getResources().getDisplayMetrics().density);
    }

    private void buildUi() {
        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Color.parseColor("#121A3C"));

        web = new WebView(this);
        web.setBackgroundColor(Color.parseColor("#121A3C"));
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(true);
        s.setSupportZoom(false);
        s.setBuiltInZoomControls(false);
        s.setMediaPlaybackRequiresUserGesture(true);
        s.setUserAgentString(s.getUserAgentString() + " UniHubApp/" + appVersion());
        web.addJavascriptInterface(new Bridge(), "UniHubApp");
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest req) {
                Uri u = req.getUrl();
                String h = u.getHost();
                if ("127.0.0.1".equals(h) || "localhost".equals(h)) return false;
                try {
                    startActivity(new Intent(Intent.ACTION_VIEW, u));
                } catch (ActivityNotFoundException ignored) {
                }
                return true;
            }

            @Override
            public void onPageFinished(WebView v, String url) {
                if (url != null && url.startsWith(Engine.BASE)) {
                    pageLoaded = true;
                    splash.animate().alpha(0f).setDuration(250).withEndAction(() -> splash.setVisibility(View.GONE));
                }
            }

            @Override
            public void onReceivedError(WebView v, WebResourceRequest req, WebResourceError err) {
                if (req.isForMainFrame()) {
                    // the engine restarted (update, crash): wait for it and load again
                    showSplash("جارٍ تشغيل البرنامج…", false);
                    waitForEngine();
                }
            }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView v, ValueCallback<Uri[]> cb, FileChooserParams params) {
                if (fileCallback != null) fileCallback.onReceiveValue(null);
                fileCallback = cb;
                Intent i = new Intent(Intent.ACTION_GET_CONTENT).addCategory(Intent.CATEGORY_OPENABLE);
                String[] types = params.getAcceptTypes();
                i.setType(types.length > 0 && types[0] != null && !types[0].isEmpty() && types[0].contains("/") ? types[0] : "image/*");
                if (params.getMode() == FileChooserParams.MODE_OPEN_MULTIPLE) i.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
                try {
                    startActivityForResult(Intent.createChooser(i, "اختر الصور"), REQ_FILES);
                } catch (ActivityNotFoundException e) {
                    fileCallback = null;
                    return false;
                }
                return true;
            }
        });
        root.addView(web, new FrameLayout.LayoutParams(-1, -1));

        splash = new LinearLayout(this);
        splash.setOrientation(LinearLayout.VERTICAL);
        splash.setGravity(Gravity.CENTER);
        splash.setBackgroundColor(Color.parseColor("#121A3C"));
        ImageView logo = new ImageView(this);
        logo.setImageResource(R.mipmap.ic_launcher);
        splash.addView(logo, new LinearLayout.LayoutParams(dp(96), dp(96)));
        TextView name = new TextView(this);
        name.setText(R.string.app_name);
        name.setTextColor(Color.WHITE);
        name.setTextSize(22);
        name.setTypeface(Typeface.DEFAULT_BOLD);
        name.setPadding(0, dp(18), 0, dp(6));
        splash.addView(name);
        splashText = new TextView(this);
        splashText.setTextColor(Color.parseColor("#CDB27A"));
        splashText.setTextSize(14);
        splashText.setGravity(Gravity.CENTER);
        splashText.setPadding(dp(32), 0, dp(32), dp(18));
        splash.addView(splashText);
        ProgressBar pb = new ProgressBar(this);
        pb.setIndeterminate(true);
        splash.addView(pb, new LinearLayout.LayoutParams(dp(28), dp(28)));
        retry = new Button(this);
        retry.setText("إعادة المحاولة");
        retry.setVisibility(View.GONE);
        retry.setOnClickListener(v -> {
            EngineService.start(this);
            showSplash("جارٍ تشغيل البرنامج…", false);
            waitForEngine();
        });
        splash.addView(retry);
        root.addView(splash, new FrameLayout.LayoutParams(-1, -1));
        showSplash("جارٍ تشغيل البرنامج…", false);
        setContentView(root);
    }

    private void showSplash(String text, boolean failed) {
        splash.animate().cancel();
        splash.setAlpha(1f);
        splash.setVisibility(View.VISIBLE);
        splashText.setText(text);
        retry.setVisibility(failed ? View.VISIBLE : View.GONE);
    }

    private volatile boolean waiting;

    private void waitForEngine() {
        if (waiting) return;
        waiting = true;
        new Thread(() -> {
            boolean ok = false;
            for (int i = 0; i < 120 && !destroyed; i++) { // up to 60 seconds
                if (Engine.alive()) {
                    ok = true;
                    break;
                }
                try {
                    Thread.sleep(500);
                } catch (InterruptedException e) {
                    break;
                }
            }
            waiting = false;
            final boolean ready = ok;
            runOnUiThread(() -> {
                if (destroyed) return;
                if (ready) web.loadUrl(Engine.BASE + "?k=" + Engine.hostKey(this));
                else showSplash("لم يبدأ البرنامج. تأكد من وجود مساحة كافية على الهاتف ثم أعد المحاولة.", true);
            });
        }, "engine-wait").start();
    }

    private String appVersion() {
        try {
            return getPackageManager().getPackageInfo(getPackageName(), 0).versionName;
        } catch (Exception e) {
            return "";
        }
    }

    // ---------------------------------------------------------------- file chooser

    @Override
    protected void onActivityResult(int req, int result, Intent data) {
        super.onActivityResult(req, result, data);
        if (req != REQ_FILES || fileCallback == null) return;
        Uri[] uris = null;
        if (result == RESULT_OK && data != null) {
            ClipData clip = data.getClipData();
            if (clip != null) {
                uris = new Uri[clip.getItemCount()];
                for (int i = 0; i < uris.length; i++) uris[i] = clip.getItemAt(i).getUri();
            } else if (data.getData() != null) {
                uris = new Uri[]{data.getData()};
            }
        }
        fileCallback.onReceiveValue(uris);
        fileCallback = null;
    }

    // ---------------------------------------------------------------- updates

    private boolean canInstall() {
        return getPackageManager().canRequestPackageInstalls();
    }

    /** Opens the system installer for a downloaded, verified update. */
    void requestInstall(File apk) {
        runOnUiThread(() -> {
            if (!apk.isFile()) return;
            if (canInstall()) {
                startInstaller(apk);
                return;
            }
            pendingInstall = apk;
            new AlertDialog.Builder(this)
                    .setTitle("خطوة واحدة لتفعيل التحديثات")
                    .setMessage("حتى يستطيع منظّم الجامعة تثبيت تحديثاته، فعّل خيار «السماح من هذا المصدر» في الصفحة التالية ثم ارجع للتطبيق. تحتاج هذا مرة واحدة فقط.")
                    .setPositiveButton("متابعة", (d, w) -> {
                        try {
                            startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + getPackageName())));
                        } catch (ActivityNotFoundException e) {
                            Toast.makeText(this, "افتح الإعدادات وفعّل التثبيت من منظّم الجامعة", Toast.LENGTH_LONG).show();
                        }
                    })
                    .setNegativeButton("لاحقاً", (d, w) -> pendingInstall = null)
                    .show();
        });
    }

    private void startInstaller(File apk) {
        Uri u = FilesProvider.uriFor(this, apk);
        if (u == null) return;
        Intent i = new Intent(Intent.ACTION_VIEW).setDataAndType(u, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        try {
            startActivity(i);
        } catch (ActivityNotFoundException e) {
            Toast.makeText(this, "تعذر فتح أداة التثبيت", Toast.LENGTH_LONG).show();
        }
    }

    // ---------------------------------------------------------------- page bridge

    /** Small bridge the page uses to match the phone's bars to the app's theme. */
    private class Bridge {
        @JavascriptInterface
        public String version() {
            return appVersion();
        }

        @JavascriptInterface
        public void setBars(String color, boolean darkTheme) {
            runOnUiThread(() -> {
                try {
                    int c = Color.parseColor(color);
                    Window w = getWindow();
                    w.setStatusBarColor(c);
                    w.setNavigationBarColor(c);
                    web.setBackgroundColor(c);
                    if (Build.VERSION.SDK_INT >= 30) {
                        WindowInsetsController ic = w.getInsetsController();
                        if (ic != null) {
                            int mask = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
                            ic.setSystemBarsAppearance(darkTheme ? 0 : mask, mask);
                        }
                    } else {
                        View d = w.getDecorView();
                        int f = d.getSystemUiVisibility();
                        int light = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
                        d.setSystemUiVisibility(darkTheme ? f & ~light : f | light);
                    }
                } catch (Exception ignored) {
                }
            });
        }
    }
}
