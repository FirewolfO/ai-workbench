package shop.lxvb.aiworkbench;

import android.app.Activity;
import android.app.DownloadManager;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Environment;
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

public final class MainActivity extends Activity {
    private static final String APP_URL = "https://ai.lxvb.top";
    private static final int FILE_CHOOSER_REQUEST = 4102;

    private WebView webView;
    private ValueCallback<Uri[]> fileCallback;

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
        super.onDestroy();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
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
            if (("https".equals(uri.getScheme()) && "ai.lxvb.top".equals(host)) || "10.251.237.216".equals(host)) {
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
