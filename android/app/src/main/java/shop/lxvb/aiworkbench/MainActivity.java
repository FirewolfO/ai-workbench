package shop.lxvb.aiworkbench;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.DownloadManager;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.Context;
import android.content.Intent;
import android.graphics.Color;
import android.graphics.Insets;
import android.net.Uri;
import android.os.Bundle;
import android.os.Environment;
import android.provider.Settings;
import android.util.Log;
import android.view.Gravity;
import android.webkit.CookieManager;
import android.webkit.DownloadListener;
import android.webkit.JavascriptInterface;
import android.webkit.MimeTypeMap;
import android.webkit.SafeBrowsingResponse;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.widget.Button;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.File;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final String TAG = "AIWorkbench";
    private static final String APP_URL = "https://ai.lxvb.top";
    static final String UPDATE_PREFERENCES = "app_update";
    private static final int FILE_CHOOSER_REQUEST = 4102;
    private static final int INSTALL_PERMISSION_REQUEST = 4103;
    private static final long UPDATE_CHECK_INTERVAL_MS = 15 * 60 * 1000L;

    private WebView webView;
    private ValueCallback<Uri[]> fileCallback;
    private final ExecutorService networkExecutor = Executors.newSingleThreadExecutor();
    private AppUpdate pendingUpdate;
    private AppUpdate availableUpdate;
    private AppUpdate latestRelease;
    private boolean checkingForUpdate;
    private boolean manualUpdateCheck;
    private long lastUpdateCheckAt;
    private String promptedVersion = "";
    private String updateStatus = "idle";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Color.WHITE);
        root.setOnApplyWindowInsetsListener((view, windowInsets) -> {
            Insets safeArea = windowInsets.getInsets(
                    WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
            if (view.getPaddingLeft() != safeArea.left || view.getPaddingTop() != safeArea.top
                    || view.getPaddingRight() != safeArea.right || view.getPaddingBottom() != safeArea.bottom) {
                view.setPadding(safeArea.left, safeArea.top, safeArea.right, safeArea.bottom);
            }
            return WindowInsets.CONSUMED;
        });
        setContentView(root);
        // PhoneWindow has no DecorView before setContentView on Android 15 and 16.
        getWindow().setNavigationBarContrastEnforced(false);
        root.requestApplyInsets();

        try {
            webView = new WebView(this);
            webView.setBackgroundColor(Color.rgb(244, 246, 245));
            root.addView(webView, new FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
            configureWebView();
        } catch (RuntimeException error) {
            showStartupError(root, error);
            checkForUpdate();
            return;
        }
        getOnBackInvokedDispatcher().registerOnBackInvokedCallback(0, this::handleBack);

        if (savedInstanceState == null) {
            webView.loadUrl(APP_URL);
        } else {
            webView.restoreState(savedInstanceState);
        }
        checkForUpdate();
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (System.currentTimeMillis() - lastUpdateCheckAt >= UPDATE_CHECK_INTERVAL_MS) {
            checkForUpdate();
        }
    }

    private void showStartupError(FrameLayout root, RuntimeException error) {
        Log.e(TAG, "Unable to initialize the system WebView", error);
        if (webView != null) {
            root.removeView(webView);
            webView.destroy();
            webView = null;
        }

        int spacing = Math.round(24 * getResources().getDisplayMetrics().density);
        LinearLayout message = new LinearLayout(this);
        message.setOrientation(LinearLayout.VERTICAL);
        message.setGravity(Gravity.CENTER);
        message.setPadding(spacing, spacing, spacing, spacing);

        TextView description = new TextView(this);
        description.setText(R.string.webview_startup_failed);
        description.setTextColor(Color.rgb(24, 33, 31));
        description.setTextSize(16);
        description.setGravity(Gravity.CENTER);
        message.addView(description, new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        Button retry = new Button(this);
        retry.setText(R.string.retry);
        retry.setOnClickListener(view -> recreate());
        LinearLayout.LayoutParams retryLayout = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        retryLayout.topMargin = spacing;
        message.addView(retry, retryLayout);
        root.addView(message, new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
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
        webView.addJavascriptInterface(new WorkbenchNativeBridge(), "AIWorkbenchNative");
        webView.setWebViewClient(new WorkbenchWebViewClient());
        webView.setWebChromeClient(new WorkbenchChromeClient());
        webView.setDownloadListener(this::download);
    }

    private void handleBack() {
        if (webView != null && webView.canGoBack()) {
            webView.goBack();
        } else {
            finish();
        }
    }

    @Override
    protected void onSaveInstanceState(Bundle state) {
        if (webView != null) webView.saveState(state);
        super.onSaveInstanceState(state);
    }

    @Override
    protected void onDestroy() {
        if (fileCallback != null) {
            fileCallback.onReceiveValue(null);
            fileCallback = null;
        }
        if (webView != null) {
            webView.stopLoading();
            webView.setWebChromeClient(null);
            webView.setWebViewClient(null);
            webView.destroy();
            webView = null;
        }
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
        Uri[] selectedFiles = resolveSelectedFiles(resultCode, data);
        fileCallback.onReceiveValue(selectedFiles);
        fileCallback = null;
        if (resultCode == RESULT_OK && (selectedFiles == null || selectedFiles.length == 0)) {
            Toast.makeText(this, R.string.file_selection_failed, Toast.LENGTH_LONG).show();
        }
    }

    private Uri[] resolveSelectedFiles(int resultCode, Intent data) {
        if (resultCode != RESULT_OK || data == null) return null;

        Set<Uri> selected = new LinkedHashSet<>();
        ClipData clipData = data.getClipData();
        if (clipData != null) {
            for (int index = 0; index < clipData.getItemCount(); index++) {
                Uri uri = clipData.getItemAt(index).getUri();
                if (uri != null) selected.add(uri);
            }
        }

        try {
            ArrayList<Uri> streams = data.getParcelableArrayListExtra(Intent.EXTRA_STREAM, Uri.class);
            if (streams != null) {
                for (Uri uri : streams) {
                    if (uri != null) selected.add(uri);
                }
            }
            Uri stream = data.getParcelableExtra(Intent.EXTRA_STREAM, Uri.class);
            if (stream != null) selected.add(stream);
        } catch (RuntimeException ignored) {
            // Some vendor pickers use a different parcelable shape for EXTRA_STREAM.
        }

        Uri direct = data.getData();
        if (direct != null) selected.add(direct);
        try {
            Uri[] parsed = WebChromeClient.FileChooserParams.parseResult(resultCode, data);
            if (parsed != null) {
                for (Uri uri : parsed) {
                    if (uri != null) selected.add(uri);
                }
            }
        } catch (RuntimeException ignored) {
            // Explicitly parsed ClipData remains usable when the WebView helper rejects vendor data.
        }
        return selected.isEmpty() ? null : selected.toArray(new Uri[0]);
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
        checkForUpdate(false);
    }

    private void checkForUpdate(boolean userInitiated) {
        if (getSharedPreferences(UPDATE_PREFERENCES, MODE_PRIVATE).getLong("update_download_id", -1) >= 0) {
            updateStatus = "downloading";
            notifyWebUpdateStatus();
            return;
        }
        if (checkingForUpdate) {
            manualUpdateCheck |= userInitiated;
            notifyWebUpdateStatus();
            return;
        }
        manualUpdateCheck = userInitiated;
        checkingForUpdate = true;
        updateStatus = "checking";
        lastUpdateCheckAt = System.currentTimeMillis();
        notifyWebUpdateStatus();
        networkExecutor.execute(() -> {
            AppUpdate update = null;
            boolean success = false;
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
                    success = update != null && update.isValid();
                }
                connection.disconnect();
            } catch (Exception ignored) {
                // Version checks are best-effort and must never block the workbench.
            }
            AppUpdate result = update;
            boolean checked = success;
            runOnUiThread(() -> {
                boolean requestedByUser = manualUpdateCheck;
                manualUpdateCheck = false;
                checkingForUpdate = false;
                if (!checked) {
                    updateStatus = "error";
                    notifyWebUpdateStatus();
                    return;
                }
                latestRelease = result;
                if (AppUpdate.isNewer(result.version, BuildConfig.VERSION_NAME)) {
                    availableUpdate = result;
                    updateStatus = "available";
                    notifyWebUpdateStatus();
                    notifyWebUpdate();
                    if (requestedByUser || !result.version.equals(promptedVersion)) {
                        promptedVersion = result.version;
                        showUpdatePrompt(result);
                    }
                } else {
                    availableUpdate = null;
                    updateStatus = "current";
                    notifyWebUpdateStatus();
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

    private void notifyWebUpdate() {
        if (webView == null || availableUpdate == null) return;
        try {
            JSONObject detail = new JSONObject()
                    .put("version", availableUpdate.version)
                    .put("size", availableUpdate.size);
            webView.evaluateJavascript("window.dispatchEvent(new CustomEvent('ai-workbench-app-update',{detail:"
                    + detail + "}));", null);
        } catch (Exception ignored) {
            // The native dialog remains available if the web page is being recreated.
        }
    }

    private void notifyWebUpdateStatus() {
        if (webView == null) return;
        try {
            AppUpdate release = availableUpdate != null ? availableUpdate : latestRelease;
            JSONObject detail = new JSONObject()
                    .put("status", updateStatus)
                    .put("currentVersion", BuildConfig.VERSION_NAME);
            if (release != null) {
                detail.put("latestVersion", release.version).put("size", release.size);
            }
            webView.evaluateJavascript("window.dispatchEvent(new CustomEvent('ai-workbench-app-update-status',{detail:"
                    + detail + "}));", null);
        } catch (Exception ignored) {
            // Version state is also retained natively for the next page-ready callback.
        }
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
            updateStatus = "downloading";
            notifyWebUpdateStatus();
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
                Intent chooser = params.createIntent();
                chooser.addCategory(Intent.CATEGORY_OPENABLE);
                chooser.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
                if (params.getMode() == FileChooserParams.MODE_OPEN_MULTIPLE) {
                    chooser.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
                }
                startActivityForResult(chooser, FILE_CHOOSER_REQUEST);
                return true;
            } catch (ActivityNotFoundException error) {
                fileCallback = null;
                Toast.makeText(MainActivity.this, R.string.file_picker_missing, Toast.LENGTH_SHORT).show();
                return false;
            }
        }
    }

    private final class WorkbenchNativeBridge {
        @JavascriptInterface
        public void checkForUpdate() {
            runOnUiThread(() -> MainActivity.this.checkForUpdate(true));
        }

        @JavascriptInterface
        public void getUpdateStatus() {
            runOnUiThread(() -> {
                notifyWebUpdateStatus();
                if (availableUpdate != null) notifyWebUpdate();
            });
        }
    }

    private final class WorkbenchWebViewClient extends WebViewClient {
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            Uri uri = request.getUrl();
            String host = uri.getHost();
            if ("ai-workbench".equals(uri.getScheme()) && "update".equals(host)) {
                if (availableUpdate != null) showUpdatePrompt(availableUpdate);
                else checkForUpdate(true);
                return true;
            }
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
        public void onPageFinished(WebView view, String url) {
            super.onPageFinished(view, url);
            if (url != null && url.startsWith(APP_URL)) {
                notifyWebUpdateStatus();
                notifyWebUpdate();
            }
        }

        @Override
        public void onSafeBrowsingHit(WebView view, WebResourceRequest request, int threatType, SafeBrowsingResponse callback) {
            callback.backToSafety(true);
            Toast.makeText(MainActivity.this, R.string.unsafe_page, Toast.LENGTH_LONG).show();
        }
    }
}
