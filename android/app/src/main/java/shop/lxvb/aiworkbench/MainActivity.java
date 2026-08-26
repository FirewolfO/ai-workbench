package shop.lxvb.aiworkbench;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.DownloadManager;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Environment;
import android.provider.Settings;
import android.webkit.CookieManager;
import android.webkit.DownloadListener;
import android.webkit.MimeTypeMap;
import android.webkit.SafeBrowsingResponse;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.File;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final String APP_URL = "https://ai.lxvb.top";
    static final String UPDATE_PREFERENCES = "app_update";
    private static final int FILE_CHOOSER_REQUEST = 4102;
    private static final int INSTALL_PERMISSION_REQUEST = 4103;

    private WebView webView;
    private ValueCallback<Uri[]> fileCallback;
    private final ExecutorService networkExecutor = Executors.newSingleThreadExecutor();
    private AppUpdate pendingUpdate;
    private boolean checkingForUpdate;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().setStatusBarColor(Color.rgb(22, 53, 47));
        getWindow().setNavigationBarColor(Color.WHITE);

        webView = new WebView(this);
        webView.setBackgroundColor(Color.rgb(244, 246, 245));
        setContentView(webView);
        configureWebView();
        getOnBackInvokedDispatcher().registerOnBackInvokedCallback(0, this::handleBack);

        if (savedInstanceState == null) {
            webView.loadUrl(APP_URL);
        } else {
            webView.restoreState(savedInstanceState);
        }
        checkForUpdate();
    }

    private void configureWebView() {
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(true);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setMediaPlaybackRequiresUserGesture(true);
        settings.setSupportZoom(false);
        settings.setUserAgentString(settings.getUserAgentString() + " AIWorkbenchAndroid/" + BuildConfig.VERSION_NAME);

        CookieManager.getInstance().setAcceptCookie(true);
        CookieManager.getInstance().setAcceptThirdPartyCookies(webView, false);
        WebView.startSafeBrowsing(this, null);
        webView.setWebViewClient(new WorkbenchWebViewClient());
        webView.setWebChromeClient(new WorkbenchChromeClient());
        webView.setDownloadListener(this::download);
    }

    private void handleBack() {
        if (webView.canGoBack()) {
            webView.goBack();
        } else {
            finish();
        }
    }

    @Override
    protected void onSaveInstanceState(Bundle state) {
        webView.saveState(state);
        super.onSaveInstanceState(state);
    }

    @Override
    protected void onDestroy() {
        if (fileCallback != null) {
            fileCallback.onReceiveValue(null);
            fileCallback = null;
        }
        webView.stopLoading();
        webView.setWebChromeClient(null);
        webView.setWebViewClient(null);
        webView.destroy();
        networkExecutor.shutdownNow();
        super.onDestroy();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == INSTALL_PERMISSION_REQUEST) {
            if (getPackageManager().canRequestPackageInstalls()) {
                downloadPendingUpdate();
            } else {
                Toast.makeText(this, R.string.update_permission_required, Toast.LENGTH_LONG).show();
            }
            return;
        }
        if (requestCode != FILE_CHOOSER_REQUEST || fileCallback == null) {
            return;
        }
        fileCallback.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(resultCode, data));
        fileCallback = null;
    }

    private void download(String url, String userAgent, String disposition, String mimeType, long size) {
        try {
            DownloadManager.Request request = new DownloadManager.Request(Uri.parse(url));
            request.addRequestHeader("User-Agent", userAgent);
            request.addRequestHeader("Cookie", CookieManager.getInstance().getCookie(url));
            request.setMimeType(mimeType);
            request.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
            String extension = MimeTypeMap.getSingleton().getExtensionFromMimeType(mimeType);
            request.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, "ai-workbench-" + System.currentTimeMillis() + (extension == null ? "" : "." + extension));
            ((DownloadManager) getSystemService(Context.DOWNLOAD_SERVICE)).enqueue(request);
            Toast.makeText(this, R.string.download_started, Toast.LENGTH_SHORT).show();
        } catch (RuntimeException error) {
            Toast.makeText(this, R.string.download_failed, Toast.LENGTH_SHORT).show();
        }
    }

    private void checkForUpdate() {
        if (checkingForUpdate || getSharedPreferences(UPDATE_PREFERENCES, MODE_PRIVATE).getLong("update_download_id", -1) >= 0) {
            return;
        }
        checkingForUpdate = true;
        networkExecutor.execute(() -> {
            AppUpdate update = null;
            try {
                URL endpoint = new URL(BuildConfig.APP_CENTER_URL + "/api/apps/ai-workbench/latest");
                HttpURLConnection connection = (HttpURLConnection) endpoint.openConnection();
                connection.setConnectTimeout(8000);
                connection.setReadTimeout(8000);
                connection.setRequestProperty("Accept", "application/json");
                connection.setRequestProperty("User-Agent", "AIWorkbenchAndroid/" + BuildConfig.VERSION_NAME);
                if (connection.getResponseCode() == 200) {
                    try (InputStream body = connection.getInputStream()) {
                        JSONObject release = new JSONObject(new String(body.readAllBytes(), StandardCharsets.UTF_8))
                                .optJSONObject("release");
                        if (release != null) update = AppUpdate.from(release);
                    }
                }
                connection.disconnect();
            } catch (Exception ignored) {
                // Version checks are best-effort and must never block the workbench.
            }
            AppUpdate result = update;
            runOnUiThread(() -> {
                checkingForUpdate = false;
                if (result != null && result.isValid() && AppUpdate.isNewer(result.version, BuildConfig.VERSION_NAME)) {
                    showUpdatePrompt(result);
                }
            });
        });
    }

    private void showUpdatePrompt(AppUpdate update) {
        if (isFinishing() || isDestroyed()) return;
        String size = update.size > 0 ? "\n安装包大小：" + formatSize(update.size) : "";
        new AlertDialog.Builder(this)
                .setTitle("发现新版本 " + update.version)
                .setMessage("可以下载并覆盖安装最新版 AI 工作台。" + size)
                .setNegativeButton("稍后", null)
                .setPositiveButton("立即更新", (dialog, which) -> prepareUpdate(update))
                .show();
    }

    private void prepareUpdate(AppUpdate update) {
        pendingUpdate = update;
        if (!getPackageManager().canRequestPackageInstalls()) {
            startActivityForResult(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                    Uri.parse("package:" + getPackageName())), INSTALL_PERMISSION_REQUEST);
            return;
        }
        downloadPendingUpdate();
    }

    private void downloadPendingUpdate() {
        AppUpdate update = pendingUpdate;
        if (update == null) return;
        pendingUpdate = null;
        String filename = update.filename.replaceAll("[^A-Za-z0-9._-]", "_");
        if (!filename.toLowerCase(Locale.ROOT).endsWith(".apk")) filename += ".apk";
        String url = update.url.startsWith("http://") || update.url.startsWith("https://")
                ? update.url : BuildConfig.APP_CENTER_URL + (update.url.startsWith("/") ? "" : "/") + update.url;

        DownloadManager.Request request = new DownloadManager.Request(Uri.parse(url))
                .setTitle("AI 工作台 " + update.version)
                .setDescription("正在下载安装包")
                .setMimeType("application/vnd.android.package-archive")
                .setAllowedNetworkTypes(DownloadManager.Request.NETWORK_WIFI | DownloadManager.Request.NETWORK_MOBILE)
                .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
        File downloadDir = getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS);
        if (downloadDir != null) {
            File previous = new File(downloadDir, filename);
            if (previous.exists() && !previous.delete()) {
                Toast.makeText(this, R.string.update_replace_failed, Toast.LENGTH_LONG).show();
                return;
            }
            request.setDestinationInExternalFilesDir(this, Environment.DIRECTORY_DOWNLOADS, filename);
        }
        try {
            long id = ((DownloadManager) getSystemService(DOWNLOAD_SERVICE)).enqueue(request);
            getSharedPreferences(UPDATE_PREFERENCES, MODE_PRIVATE).edit().putLong("update_download_id", id).apply();
            Toast.makeText(this, R.string.update_started, Toast.LENGTH_LONG).show();
        } catch (RuntimeException error) {
            Toast.makeText(this, R.string.download_failed, Toast.LENGTH_LONG).show();
        }
    }

    private String formatSize(long bytes) {
        if (bytes < 1024L * 1024L) return Math.max(1, bytes / 1024L) + " KB";
        return String.format(Locale.CHINA, "%.1f MB", bytes / 1024d / 1024d);
    }

    private final class WorkbenchChromeClient extends WebChromeClient {
        @Override
        public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
            if (fileCallback != null) {
                fileCallback.onReceiveValue(null);
            }
            fileCallback = callback;
            try {
                startActivityForResult(params.createIntent(), FILE_CHOOSER_REQUEST);
                return true;
            } catch (ActivityNotFoundException error) {
                fileCallback = null;
                Toast.makeText(MainActivity.this, R.string.file_picker_missing, Toast.LENGTH_SHORT).show();
                return false;
            }
        }
    }

    private final class WorkbenchWebViewClient extends WebViewClient {
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            Uri uri = request.getUrl();
            String host = uri.getHost();
            if ("https".equals(uri.getScheme()) && ("ai.lxvb.top".equals(host) || "people.lxvb.top".equals(host))) {
                return false;
            }
            try {
                startActivity(new Intent(Intent.ACTION_VIEW, uri));
            } catch (ActivityNotFoundException error) {
                Toast.makeText(MainActivity.this, R.string.link_failed, Toast.LENGTH_SHORT).show();
            }
            return true;
        }

        @Override
        public void onSafeBrowsingHit(WebView view, WebResourceRequest request, int threatType, SafeBrowsingResponse callback) {
            callback.backToSafety(true);
            Toast.makeText(MainActivity.this, R.string.unsafe_page, Toast.LENGTH_LONG).show();
        }
    }
}
