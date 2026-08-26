package shop.lxvb.aiworkbench;

import android.app.DownloadManager;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.database.Cursor;
import android.widget.Toast;

public final class UpdateDownloadReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (!DownloadManager.ACTION_DOWNLOAD_COMPLETE.equals(intent.getAction())) return;
        long id = intent.getLongExtra(DownloadManager.EXTRA_DOWNLOAD_ID, -1);
        long expected = context.getSharedPreferences(MainActivity.UPDATE_PREFERENCES, Context.MODE_PRIVATE)
                .getLong(MainActivity.UPDATE_DOWNLOAD_ID, -1);
        if (id < 0 || id != expected) return;

        DownloadManager manager = (DownloadManager) context.getSystemService(Context.DOWNLOAD_SERVICE);
        int status = DownloadManager.STATUS_FAILED;
        try (Cursor cursor = manager.query(new DownloadManager.Query().setFilterById(id))) {
            if (cursor.moveToFirst()) {
                status = cursor.getInt(cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS));
            }
        }
        if (status != DownloadManager.STATUS_SUCCESSFUL) {
            context.getSharedPreferences(MainActivity.UPDATE_PREFERENCES, Context.MODE_PRIVATE)
                    .edit().remove(MainActivity.UPDATE_DOWNLOAD_ID).remove(MainActivity.UPDATE_READY_ID).apply();
            Toast.makeText(context, R.string.update_download_failed, Toast.LENGTH_LONG).show();
            return;
        }
        context.getSharedPreferences(MainActivity.UPDATE_PREFERENCES, Context.MODE_PRIVATE)
                .edit().remove(MainActivity.UPDATE_DOWNLOAD_ID).putLong(MainActivity.UPDATE_READY_ID, id).apply();
    }
}
