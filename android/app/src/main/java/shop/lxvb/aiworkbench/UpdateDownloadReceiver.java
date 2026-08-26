package shop.lxvb.aiworkbench;

import android.app.DownloadManager;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.database.Cursor;
import android.net.Uri;
import android.widget.Toast;

public final class UpdateDownloadReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (!DownloadManager.ACTION_DOWNLOAD_COMPLETE.equals(intent.getAction())) return;
        long id = intent.getLongExtra(DownloadManager.EXTRA_DOWNLOAD_ID, -1);
        long expected = context.getSharedPreferences(MainActivity.UPDATE_PREFERENCES, Context.MODE_PRIVATE)
                .getLong("update_download_id", -1);
        if (id < 0 || id != expected) return;

        DownloadManager manager = (DownloadManager) context.getSystemService(Context.DOWNLOAD_SERVICE);
        int status = DownloadManager.STATUS_FAILED;
        try (Cursor cursor = manager.query(new DownloadManager.Query().setFilterById(id))) {
            if (cursor.moveToFirst()) {
                status = cursor.getInt(cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS));
            }
        }
        context.getSharedPreferences(MainActivity.UPDATE_PREFERENCES, Context.MODE_PRIVATE)
                .edit().remove("update_download_id").apply();
        if (status != DownloadManager.STATUS_SUCCESSFUL) {
            Toast.makeText(context, R.string.update_download_failed, Toast.LENGTH_LONG).show();
            return;
        }
        Uri apk = manager.getUriForDownloadedFile(id);
        if (apk == null) {
            Toast.makeText(context, R.string.update_open_failed, Toast.LENGTH_LONG).show();
            return;
        }
        try {
            context.startActivity(new Intent(Intent.ACTION_VIEW)
                    .setDataAndType(apk, "application/vnd.android.package-archive")
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_GRANT_READ_URI_PERMISSION));
        } catch (RuntimeException error) {
            Toast.makeText(context, R.string.update_open_failed, Toast.LENGTH_LONG).show();
        }
    }
}
